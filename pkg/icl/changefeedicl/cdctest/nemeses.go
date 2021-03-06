// Copyright 2019  The Cockroach Authors.
//
// Licensed as a Cockroach Enterprise file under the ZNBase Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/znbasedb/znbase/blob/master/licenses/ICL.txt

package cdctest

import (
	"context"
	gosql "database/sql"
	gojson "encoding/json"
	"fmt"
	"math/rand"
	"strings"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/util/fsm"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/randutil"
)

// RunNemesis runs a jepsen-style validation of whether a cdc meets our
// user-facing guarantees. It's driven by a state machine with various nemeses:
// txn begin/commit/rollback, job pause/unpause.
//
// CDCs have a set of user-facing guarantees about ordering and
// duplicates, which the two cdctest.Validator implementations verify for the
// real output of a cdc. The output rows and resolved timestamps of the
// tested feed are fed into them to check for anomalies.
func RunNemesis(f TestFeedFactory, db *gosql.DB) (Validator, error) {
	// possible additional nemeses:
	// - schema changes
	// - merges
	// - rebalancing
	// - lease transfers
	// - receiving snapshots
	// mostly redundant with the pause/unpause nemesis, but might be nice to have:
	// - znbase chaos
	// - sink chaos

	ctx := context.Background()
	rng, _ := randutil.NewPseudoRand()

	ns := &nemeses{
		rowCount: 4,
		db:       db,
		// eventMix does not have to add to 100
		eventMix: map[fsm.Event]int{
			// eventTransact opens an UPSERT transaction is there is not one open. If
			// there is one open, it either commits it or rolls it back.
			eventTransact{}: 45,

			// eventFeedMessage reads a message from the feed, or if the state machine
			// thinks there will be no message available, it falls back to
			// eventTransact.
			eventFeedMessage{}: 45,

			// eventPause PAUSEs the cdc. The state machine will handle
			// RESUMEing it.
			// TODO(dan): This deadlocks eventPause{}: 10,

			// eventPush pushes every open transaction by running a high priority
			// SELECT.
			// TODO(dan): This deadlocks eventPush{}: 30,

			// eventAbort aborts every open transaction by running a high priority
			// DELETE.
			// TODO(dan): This deadlocks eventAbort{}: 30,

			// eventSplit splits between two random rows (the split is a no-op if it
			// already exists).
			// TODO(dan): This deadlocks eventSplit{}: 10,
		},
	}

	// Create the table and set up some initial splits.
	if _, err := db.Exec(`CREATE TABLE foo (id INT PRIMARY KEY, ts STRING DEFAULT '0')`); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`SET CLUSTER SETTING kv.range_merge.queue_enabled = false`); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`ALTER TABLE foo SPLIT AT VALUES ($1)`, ns.rowCount/2); err != nil {
		return nil, err
	}

	// Initialize table rows by repeatedly running the `transact` transition,
	// which randomly starts, commits, and rolls back transactions. This will
	// leave some committed rows and maybe an outstanding intent for the initial
	// scan.
	for i := 0; i < ns.rowCount*5; i++ {
		if err := transact(fsm.Args{Ctx: ctx, Extended: ns}); err != nil {
			return nil, err
		}
	}

	foo, err := f.Feed(`CREATE CHANGEFEED FOR foo WITH updated, resolved`)
	if err != nil {
		return nil, err
	}
	ns.f = foo
	defer func() { _ = foo.Close() }()

	if _, err := db.Exec(`CREATE TABLE fprint (id INT PRIMARY KEY, ts STRING)`); err != nil {
		return nil, err
	}
	ns.v = MakeCountValidator(Validators{
		NewOrderValidator(`foo`),
		NewFingerprintValidator(db, `foo`, `fprint`, foo.Partitions()),
	})

	// Initialize the actual row count, overwriting what `transact` did.
	// `transact` has set this to the number of modified rows, which is correct
	// during cdc operation, but not for the initial scan, because some of
	// the rows may have had the same primary key.
	if err := db.QueryRow(`SELECT count(*) FROM foo`).Scan(&ns.availableRows); err != nil {
		return nil, err
	}

	// Kick everything off by reading the first message. This accomplishes two
	// things. First, it maximizes the chance that we hit an unresolved intent
	// during the initial scan. Second, it guarantees that the feed is running
	// before anything else commits, which could mess up the availableRows count
	// we just set.
	first, err := foo.Next()
	if err != nil {
		return nil, err
	}
	if err := noteFeedMessage(fsm.Args{Ctx: ctx, Extended: ns, Payload: first}); err != nil {
		return nil, err
	}
	// Now push everything to make sure the initial scan can complete, otherwise
	// we may deadlock.
	if err := push(fsm.Args{Ctx: ctx, Extended: ns}); err != nil {
		return nil, err
	}

	// Run the state machine until it finishes. Exit criteria is in `nextEvent`
	// and is based on the number of rows that have been resolved and the number
	// of resolved timestamp messages.
	m := fsm.MakeMachine(txnStateTransitions, stateRunning{fsm.False}, ns)
	for {
		state := m.CurState()
		if _, ok := state.(stateDone); ok {
			return ns.v, nil
		}
		event, payload, err := ns.nextEvent(rng, state, foo)
		if err != nil {
			return nil, err
		}
		if err := m.ApplyWithPayload(ctx, event, payload); err != nil {
			return nil, err
		}
	}
}

type nemeses struct {
	rowCount int
	eventMix map[fsm.Event]int
	mixTotal int

	v  *CountValidator
	db *gosql.DB
	f  TestFeed

	availableRows int
	txn           *gosql.Tx
	openTxnID     int
	openTxnTs     string
}

// nextEvent selects the next state transition.
func (ns *nemeses) nextEvent(
	rng *rand.Rand, state fsm.State, f TestFeed,
) (fsm.Event, fsm.EventPayload, error) {
	if ns.v.NumResolvedWithRows > 5 && ns.v.NumResolvedRows > 20 {
		return eventFinished{}, nil, nil
	}

	if ns.mixTotal == 0 {
		for _, weight := range ns.eventMix {
			ns.mixTotal += weight
		}
	}

	switch state {
	case stateRunning{Paused: fsm.True}:
		return eventResume{}, nil, nil
	case stateRunning{Paused: fsm.False}:
		r, t := rng.Intn(ns.mixTotal), 0
		for event, weight := range ns.eventMix {
			t += weight
			if r >= t {
				continue
			}
			if _, ok := event.(eventFeedMessage); ok {
				break
			}
			return event, nil, nil
		}

		// If there are no available rows, transact instead of reading.
		if ns.availableRows < 1 {
			return eventTransact{}, nil, nil
		}

		m, err := f.Next()
		if err != nil {
			return nil, nil, err
		} else if m == nil {
			return nil, nil, errors.Errorf(`expected another message`)
		}
		return eventFeedMessage{}, m, nil
	default:
		return nil, nil, errors.Errorf(`unknown state: %T %s`, state, state)
	}
}

type stateRunning struct {
	Paused fsm.Bool
}
type stateDone struct{}

func (stateRunning) State() {}
func (stateDone) State()    {}

type eventTransact struct{}
type eventFeedMessage struct{}
type eventPause struct{}
type eventResume struct{}
type eventPush struct{}
type eventAbort struct{}
type eventSplit struct{}
type eventFinished struct{}

func (eventTransact) Event()    {}
func (eventFeedMessage) Event() {}
func (eventPause) Event()       {}
func (eventResume) Event()      {}
func (eventPush) Event()        {}
func (eventAbort) Event()       {}
func (eventSplit) Event()       {}
func (eventFinished) Event()    {}

var txnStateTransitions = fsm.Compile(fsm.Pattern{
	stateRunning{fsm.Any}: {
		eventFinished{}: {
			Next:   stateDone{},
			Action: logEvent(cleanup),
		},
	},
	stateRunning{fsm.False}: {
		eventTransact{}: {
			Next:   stateRunning{fsm.False},
			Action: logEvent(transact),
		},
		eventFeedMessage{}: {
			Next:   stateRunning{fsm.False},
			Action: logEvent(noteFeedMessage),
		},
		eventPause{}: {
			Next:   stateRunning{fsm.True},
			Action: logEvent(pause),
		},
		eventPush{}: {
			Next:   stateRunning{fsm.True},
			Action: logEvent(push),
		},
		eventAbort{}: {
			Next:   stateRunning{fsm.True},
			Action: logEvent(abort),
		},
		eventSplit{}: {
			Next:   stateRunning{fsm.True},
			Action: logEvent(split),
		},
	},
	stateRunning{fsm.True}: {
		eventResume{}: {
			Next:   stateRunning{fsm.False},
			Action: logEvent(resume),
		},
	},
})

func logEvent(fn func(fsm.Args) error) func(fsm.Args) error {
	return func(a fsm.Args) error {
		if a.Payload == nil {
			log.Infof(a.Ctx, "%#v\n", a.Event)
		} else if m := a.Payload.(*TestFeedMessage); len(m.Resolved) > 0 {
			log.Info(a.Ctx, string(m.Resolved))
		} else {
			log.Info(a.Ctx, string(m.Value))
		}
		return fn(a)
	}
}

func cleanup(a fsm.Args) error {
	if txn := a.Extended.(*nemeses).txn; txn != nil {
		return txn.Rollback()
	}
	return nil
}

func transact(a fsm.Args) error {
	ns := a.Extended.(*nemeses)

	// If there are no transactions, create one.
	if ns.txn == nil {
		txn, err := ns.db.Begin()
		if err != nil {
			return err
		}
		if err := txn.QueryRow(
			`UPSERT INTO foo VALUES ((random() * $1)::int, cluster_logical_timestamp()::string) RETURNING *`,
			ns.rowCount,
		).Scan(&ns.openTxnID, &ns.openTxnTs); err != nil {
			return err
		}
		ns.txn = txn
		return nil
	}

	// If there is an outstanding transaction, roll it back half the time and
	// commit it the other half.
	txn := ns.txn
	ns.txn = nil

	if rand.Intn(2) < 1 {
		return txn.Rollback()
	}
	if err := txn.Commit(); err != nil && !strings.Contains(err.Error(), `restart transaction`) {
		return err
	}
	log.Infof(a.Ctx, "UPSERT (%d, %s)", ns.openTxnID, ns.openTxnTs)
	ns.availableRows++
	return nil
}

func noteFeedMessage(a fsm.Args) error {
	ns := a.Extended.(*nemeses)
	m := a.Payload.(*TestFeedMessage)
	if len(m.Resolved) > 0 {
		_, ts, err := ParseJSONValueTimestamps(m.Resolved)
		if err != nil {
			return err
		}
		return ns.v.NoteResolved(m.Partition, ts)
	}
	ts, _, err := ParseJSONValueTimestamps(m.Value)
	if err != nil {
		return err
	}

	// Some sinks, notably cloud storage sinks don't have a key, so parse it out
	// of the value.
	if m.Key == nil {
		var v struct {
			After map[string]interface{} `json:"after"`
		}
		if err := gojson.Unmarshal(m.Value, &v); err != nil {
			return err
		}
		m.Key = []byte(fmt.Sprintf("[%v]", v.After[`id`]))
	}

	ns.availableRows--
	ns.v.NoteRow(m.Partition, string(m.Key), string(m.Value), ts)
	return nil
}

func pause(a fsm.Args) error {
	return a.Extended.(*nemeses).f.Pause()
}

func resume(a fsm.Args) error {
	return a.Extended.(*nemeses).f.Resume()
}

func push(a fsm.Args) error {
	ns := a.Extended.(*nemeses)
	_, err := ns.db.Exec(`BEGIN TRANSACTION PRIORITY HIGH; SELECT * FROM foo; COMMIT`)
	return err
}

func abort(a fsm.Args) error {
	ns := a.Extended.(*nemeses)
	const delete = `BEGIN TRANSACTION PRIORITY HIGH; ` +
		`SELECT count(*) FROM [DELETE FROM foo RETURNING *]; ` +
		`COMMIT`
	var deletedRows int
	if err := ns.db.QueryRow(delete).Scan(&deletedRows); err != nil {
		return err
	}
	ns.availableRows += deletedRows
	return nil
}

func split(a fsm.Args) error {
	ns := a.Extended.(*nemeses)
	_, err := ns.db.Exec(`ALTER TABLE foo SPLIT AT VALUES ((random() * $1)::int)`, ns.rowCount)
	return err
}
