// Copyright (C) 2013-2018 by Maxim Bublis <b@codemonkey.ru>
// Use of this source code is governed by a MIT-style
// license that can be found in licenses/MIT-gofrs.txt.

// This code originated in github.com/gofrs/uuid.

package uuid

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"hash"
	"io"
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/util/syncutil"
)

// Difference in 100-nanosecond intervals between
// UUID epoch (October 15, 1582) and Unix epoch (January 1, 1970).
const epochStart = 122192928000000000

type epochFunc func() time.Time

// HWAddrFunc is the function type used to provide hardware (MAC) addresses.
type HWAddrFunc func() (net.HardwareAddr, error)

// DefaultGenerator is the default UUID Generator used by this package.
var DefaultGenerator Generator = NewGen()

// NewV1 returns a UUID based on the current timestamp and MAC address.
func NewV1() (UUID, error) {
	return DefaultGenerator.NewV1()
}

// NewV3 returns a UUID based on the MD5 hash of the namespace UUID and name.
func NewV3(ns UUID, name string) UUID {
	return DefaultGenerator.NewV3(ns, name)
}

// NewV4 returns a randomly generated UUID.
func NewV4() (UUID, error) {
	return DefaultGenerator.NewV4()
}

// NewV5 returns a UUID based on SHA-1 hash of the namespace UUID and name.
func NewV5(ns UUID, name string) UUID {
	return DefaultGenerator.NewV5(ns, name)
}

// Generator provides an interface for generating UUIDs.
type Generator interface {
	NewV1() (UUID, error)
	// NewV2(domain byte) (UUID, error) // CRL: Removed support for V2.
	NewV3(ns UUID, name string) UUID
	NewV4() (UUID, error)
	NewV5(ns UUID, name string) UUID
}

// Gen is a reference UUID generator based on the specifications laid out in
// RFC-4122 and DCE 1.1: Authentication and Security Services. This type
// satisfies the Generator interface as defined in this package.
//
// For consumers who are generating V1 UUIDs, but don't want to expose the MAC
// address of the node generating the UUIDs, the NewGenWithHWAF() function has been
// provided as a convenience. See the function's documentation for more info.
//
// The authors of this package do not feel that the majority of users will need
// to obfuscate their MAC address, and so we recommend using NewGen() to create
// a new generator.
type Gen struct {
	clockSequenceOnce sync.Once
	hardwareAddrOnce  sync.Once
	storageMutex      syncutil.Mutex

	rand io.Reader

	epochFunc     epochFunc
	hwAddrFunc    HWAddrFunc
	lastTime      uint64
	clockSequence uint16
	hardwareAddr  [6]byte
}

// interface check -- build will fail if *Gen doesn't satisfy Generator
var _ Generator = (*Gen)(nil)

// NewGen returns a new instance of Gen with some default values set. Most
// people should use this.
// NewGen by default uses crypto/rand.Reader as its source of randomness.
func NewGen() *Gen {
	return NewGenWithHWAF(defaultHWAddrFunc)
}

// NewGenWithReader returns a new instance of gen which uses r as its source of
// randomness.
func NewGenWithReader(r io.Reader) *Gen {
	g := NewGen()
	g.rand = r
	return g
}

// NewGenWithHWAF builds a new UUID generator with the HWAddrFunc provided. Most
// consumers should use NewGen() instead.
//
// This is used so that consumers can generate their own MAC addresses, for use
// in the generated UUIDs, if there is some concern about exposing the physical
// address of the machine generating the UUID.
//
// The Gen generator will only invoke the HWAddrFunc once, and cache that MAC
// address for all the future UUIDs generated by it. If you'd like to switch the
// MAC address being used, you'll need to create a new generator using this
// function.
func NewGenWithHWAF(hwaf HWAddrFunc) *Gen {
	return &Gen{
		epochFunc:  time.Now,
		hwAddrFunc: hwaf,
		rand:       rand.Reader,
	}
}

// NewV1 returns a UUID based on the current timestamp and MAC address.
func (g *Gen) NewV1() (UUID, error) {
	u := UUID{}

	timeNow, clockSeq, err := g.getClockSequence()
	if err != nil {
		return Nil, err
	}
	binary.BigEndian.PutUint32(u[0:], uint32(timeNow))
	binary.BigEndian.PutUint16(u[4:], uint16(timeNow>>32))
	binary.BigEndian.PutUint16(u[6:], uint16(timeNow>>48))
	binary.BigEndian.PutUint16(u[8:], clockSeq)

	hardwareAddr, err := g.getHardwareAddr()
	if err != nil {
		return Nil, err
	}
	copy(u[10:], hardwareAddr)

	u.SetVersion(V1)
	u.SetVariant(VariantRFC4122)

	return u, nil
}

// NewV3 returns a UUID based on the MD5 hash of the namespace UUID and name.
func (g *Gen) NewV3(ns UUID, name string) UUID {
	u := newFromHash(md5.New(), ns, name)
	u.SetVersion(V3)
	u.SetVariant(VariantRFC4122)

	return u
}

// NewV4 returns a randomly generated UUID.
func (g *Gen) NewV4() (UUID, error) {
	u := UUID{}
	if _, err := io.ReadFull(g.rand, u[:]); err != nil {
		return Nil, err
	}
	u.SetVersion(V4)
	u.SetVariant(VariantRFC4122)

	return u, nil
}

// NewV5 returns a UUID based on SHA-1 hash of the namespace UUID and name.
func (g *Gen) NewV5(ns UUID, name string) UUID {
	u := newFromHash(sha1.New(), ns, name)
	u.SetVersion(V5)
	u.SetVariant(VariantRFC4122)

	return u
}

// Returns the epoch and clock sequence.
func (g *Gen) getClockSequence() (uint64, uint16, error) {
	var err error
	g.clockSequenceOnce.Do(func() {
		buf := make([]byte, 2)
		if _, err = io.ReadFull(g.rand, buf); err != nil {
			return
		}
		g.clockSequence = binary.BigEndian.Uint16(buf)
	})
	if err != nil {
		return 0, 0, err
	}

	g.storageMutex.Lock()
	defer g.storageMutex.Unlock()

	timeNow := g.getEpoch()
	// Clock didn't change since last UUID generation.
	// Should increase clock sequence.
	if timeNow <= g.lastTime {
		g.clockSequence++
	}
	g.lastTime = timeNow

	return timeNow, g.clockSequence, nil
}

// Returns the hardware address.
func (g *Gen) getHardwareAddr() ([]byte, error) {
	var err error
	g.hardwareAddrOnce.Do(func() {
		var hwAddr net.HardwareAddr
		if hwAddr, err = g.hwAddrFunc(); err == nil {
			copy(g.hardwareAddr[:], hwAddr)
			return
		}

		// Initialize hardwareAddr randomly in case
		// of real network interfaces absence.
		if _, err = io.ReadFull(g.rand, g.hardwareAddr[:]); err != nil {
			return
		}
		// Set multicast bit as recommended by RFC-4122
		g.hardwareAddr[0] |= 0x01
	})
	if err != nil {
		return []byte{}, err
	}
	return g.hardwareAddr[:], nil
}

// Returns the difference between UUID epoch (October 15, 1582)
// and current time in 100-nanosecond intervals.
func (g *Gen) getEpoch() uint64 {
	return epochStart + uint64(g.epochFunc().UnixNano()/100)
}

// Returns the UUID based on the hashing of the namespace UUID and name.
func newFromHash(h hash.Hash, ns UUID, name string) UUID {
	u := UUID{}
	mustWrite := func(data []byte) {
		if _, err := h.Write(data); err != nil {
			panic(errors.Wrap(err, "failed to write to hash"))
		}
	}
	mustWrite(ns[:])
	mustWrite([]byte(name))
	copy(u[:], h.Sum(nil))
	return u
}

// Returns the hardware address.
func defaultHWAddrFunc() (net.HardwareAddr, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return []byte{}, err
	}
	for _, iface := range ifaces {
		if len(iface.HardwareAddr) >= 6 {
			return iface.HardwareAddr, nil
		}
	}
	return []byte{}, fmt.Errorf("uuid: no HW address found")
}
