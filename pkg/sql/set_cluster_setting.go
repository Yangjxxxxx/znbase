// Copyright 2017 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package sql

import (
	"context"
	"go/constant"
	"go/token"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/security/audit/event/infos"
	"github.com/znbasedb/znbase/pkg/security/audit/server"
	"github.com/znbasedb/znbase/pkg/server/telemetry"
	"github.com/znbasedb/znbase/pkg/settings"
	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sem/types"
	"github.com/znbasedb/znbase/pkg/sql/sqltelemetry"
	"github.com/znbasedb/znbase/pkg/sql/stats"
	"github.com/znbasedb/znbase/pkg/storage/engine"
	"github.com/znbasedb/znbase/pkg/util/humanizeutil"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/retry"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
)

// setClusterSettingNode represents a SET CLUSTER SETTING statement.
type setClusterSettingNode struct {
	name    string
	st      *cluster.Settings
	setting settings.Setting
	// If value is nil, the setting should be reset.
	value tree.TypedExpr
}

// SetClusterSetting sets session variables.
// Privileges: super user.
func (p *planner) SetClusterSetting(
	ctx context.Context, n *tree.SetClusterSetting,
) (planNode, error) {
	if err := p.RequireAdminRole(ctx, "SET CLUSTER SETTING"); err != nil {
		return nil, err
	}

	name := strings.ToLower(n.Name)
	st := p.EvalContext().Settings
	setting, ok := settings.Lookup(name)
	if !ok {
		return nil, errors.Errorf("unknown cluster setting '%s'", name)
	}

	var value tree.TypedExpr
	if n.Value != nil {
		// For DEFAULT, let the value reference be nil. That's a RESET in disguise.
		if _, ok := n.Value.(tree.DefaultVal); !ok {
			expr := n.Value
			expr = unresolvedNameToStrVal(expr)

			var requiredType types.T
			switch setting.(type) {
			case *settings.StringSetting, *settings.StateMachineSetting, *settings.ByteSizeSetting, *settings.IsolationSetting:
				requiredType = types.String
			case *settings.BoolSetting:
				requiredType = types.Bool
			case *settings.IntSetting:
				requiredType = types.Int
			case *settings.FloatSetting:
				requiredType = types.Float
			case *settings.EnumSetting:
				requiredType = types.Any
			case *settings.DurationSetting:
				requiredType = types.Interval
			default:
				return nil, errors.Errorf("unsupported setting type %T", setting)
			}

			// Special treatment of server.sql_session_timeout.
			if name == "server.sql_session_timeout" {
				if err := treatmentOfTimeout(expr); err != nil {
					return nil, err
				}
			}

			var dummyHelper tree.IndexedVarHelper
			typed, err := p.analyzeExpr(
				ctx, expr, nil, dummyHelper, requiredType, true, "SET CLUSTER SETTING "+name)
			if err != nil {
				return nil, err
			}

			value = typed
		} else if _, isStateMachineSetting := setting.(*settings.StateMachineSetting); isStateMachineSetting {
			return nil, errors.New("cannot RESET this cluster setting")
		}
	}

	if name == "engine.online.adjustable.parameters" && value != nil {
		if err := checkEngineOnlineOptions(value.String()); err != nil {
			return nil, err
		}
	}

	return &setClusterSettingNode{name: name, st: st, setting: setting, value: value}, nil
}

func treatmentOfTimeout(expr tree.Expr) error {
	switch t := expr.(type) {
	case *tree.NumVal:
		_, err := t.AsInt64()
		if err != nil && err.Error() == "numeric constant out of int64 range" {
			return err
		} else if err != nil {
			return errors.Errorf("parameter \"server.sql_session_timeout\" requires an integer value \nDETAIL: '%v' is a decimal", t)
		} else {
			if t.Negative {
				t.Negative = false
				t.Value = constant.MakeInt64(0)
				t.OrigString = "0"
			} else {
				if constant.Compare(t.Value, token.GTR, constant.MakeInt64(270)) {
					return errors.Errorf("parameter \"server.sql_session_timeout\" requires the maximum value is 270")
				}
			}
		}
	case *tree.DBool:
		return errors.Errorf("parameter \"server.sql_session_timeout\" requires an integer value \nDETAIL: '%v' is a bool", t)

	case *tree.Subquery:
		return errors.Errorf("subqueries are not allowed in SET CLUSTER SETTING")
	case *tree.StrVal:
		_, err := strconv.ParseInt(t.RawString(), 10, 64)
		if err != nil {
			return errors.Errorf("parameter \"server.sql_session_timeout\" requires an integer value \nDETAIL: '%v' is a string", t.RawString())
		}
	}
	return nil
}

func isNumeric(val string) bool {
	if val == "" {
		return false
	}
	// Trim any whitespace
	val = strings.Trim(val, " \\t\\n\\r\\v\\f")
	if val[0] == '-' || val[0] == '+' {
		if len(val) == 1 {
			return false
		}
		val = val[1:]
	}
	if val[0] == '0' && val[1] != '.' {
		return false
	}
	point, length := 0, len(val)
	for i, v := range val {
		if v == '.' { // Point
			if point > 0 || i+1 == length {
				return false
			}
			point = i
		} else if v < '0' || v > '9' {
			return false
		}
	}
	return true
}

func isBool(val string) bool {
	if val == "true" {
		return true
	} else if val == "false" {
		return true
	} else {
		return false
	}
}

// Check the parameters whether can be online update
func checkEngineOnlineOptions(params string) error {
	params = strings.Trim(params, "'")
	if params == "" {
		return nil
	}
	paramSlice := strings.Split(params, ",")
	var paramKey, paramValue string
	var opt bool
	for _, param := range paramSlice {
		paramKey = ""
		paramValue = ""
		if strings.Contains(param, "=") {
			temp := strings.Split(param, "=")
			temp[0] = strings.Replace(temp[0], " ", "", -1)
			temp[1] = strings.Replace(temp[1], " ", "", -1)
			if temp[0] == "" || temp[1] == "" {
				return errors.Errorf("Illegal parameter format, expected format: parameter = value, but obtained:%v\n", param)
			}
			for _, elOpt := range engine.ParamListOpt {
				if temp[0] == elOpt {
					paramKey = temp[0]
					paramValue = temp[1]
					opt = true
					break
				}
			}
			for _, elDBOpt := range engine.ParamListDBOpt {
				if temp[0] == elDBOpt {
					paramKey = temp[0]
					paramValue = temp[1]
					opt = false
					break
				}
			}
			if paramKey == "" {
				return errors.Errorf("unsupported engine online parameters %v", temp[0])
			}
			if opt {
				if paramKey == "report_bg_io_stats" || paramKey == "memtable_whole_key_filtering" {
					if !isBool(paramValue) {
						return errors.Errorf("Illegal value type")
					}
				} else {
					if !isNumeric(paramValue) {
						return errors.Errorf("Illegal value type")
					}
				}
			} else {
				if !isNumeric(paramValue) {
					return errors.Errorf("Illegal value type")
				}
			}
		} else {
			return errors.Errorf("Illegal parameter format, expected format: parameter = value, but obtained:%v\n", param)
		}
	}
	return nil
}

func (n *setClusterSettingNode) startExec(params runParams) error {
	if !params.p.ExtendedEvalContext().TxnImplicit {
		return errors.Errorf("SET CLUSTER SETTING cannot be used inside a transaction")
	}

	execCfg := params.extendedEvalCtx.ExecCfg
	var expectedEncodedValue string
	if err := execCfg.DB.Txn(params.ctx, func(ctx context.Context, txn *client.Txn) error {
		var reportedValue string
		if n.value == nil {
			reportedValue = "DEFAULT"
			expectedEncodedValue = n.setting.EncodedDefault()
			if _, err := execCfg.InternalExecutor.Exec(
				ctx, "reset-setting", txn,
				"DELETE FROM system.settings WHERE name = $1", n.name,
			); err != nil {
				return err
			}
		} else {
			value, err := n.value.Eval(params.p.EvalContext())
			if err != nil {
				return err
			}
			reportedValue = tree.AsStringWithFlags(value, tree.FmtBareStrings)
			var prev tree.Datum
			if _, ok := n.setting.(*settings.StateMachineSetting); ok {
				datums, err := execCfg.InternalExecutor.QueryRow(
					ctx, "retrieve-prev-setting", txn, "SELECT value FROM system.settings WHERE name = $1", n.name,
				)
				if err != nil {
					return err
				}
				if len(datums) == 0 {
					// There is a SQL migration which adds this value. If it
					// hasn't run yet, we can't update the version as we don't
					// have good enough information about the current cluster
					// version.
					return errors.New("no persisted cluster version found, please retry later")
				}
				prev = datums[0]
			}
			encoded, err := toSettingString(ctx, params.p, n.st, n.name, n.setting, value, prev)
			expectedEncodedValue = encoded
			if err != nil {
				return err
			}
			if _, err = execCfg.InternalExecutor.Exec(
				ctx, "update-setting", txn,
				`UPSERT INTO system.settings (name, value, "lastUpdated", "valueType") VALUES ($1, $2, now(), $3)`,
				n.name, encoded, n.setting.Typ(),
			); err != nil {
				return err
			}
		}

		// Report tracked cluster settings via telemetry.
		// TODO(justin): implement a more general mechanism for tracking these.
		switch n.name {
		case stats.AutoStatsClusterSettingName:
			switch expectedEncodedValue {
			case "true":
				telemetry.Inc(sqltelemetry.TurnAutoStatsOnUseCounter)
			case "false":
				telemetry.Inc(sqltelemetry.TurnAutoStatsOffUseCounter)
			}
		case ReorderJoinsLimitClusterSettingName:
			val, err := strconv.ParseInt(expectedEncodedValue, 10, 64)
			if err != nil {
				break
			}
			sqltelemetry.ReportJoinReorderLimit(int(val))
		}

		// some audit data
		params.p.curPlan.auditInfo = &server.AuditInfo{
			EventTime: timeutil.Now(),
			EventType: string(EventLogSetClusterSetting),
			TargetInfo: &server.TargetInfo{
				TargetID: 0,
				Desc: struct {
					SettingName string
					Value       string
				}{
					n.name,
					reportedValue,
				},
			},
			Info: &infos.SetClusterSettingInfo{
				SettingName: n.name,
				Value:       expectedEncodedValue,
				User:        params.SessionData().User,
			},
		}
		return nil
	}); err != nil {
		return err
	}

	if _, ok := n.setting.(*settings.StateMachineSetting); ok && n.value == nil {
		// The "version" setting doesn't have a well defined "default" since it is
		// set in a startup migration.
		return nil
	}
	errNotReady := errors.New("setting updated but timed out waiting to read new value")
	var observed string
	err := retry.ForDuration(10*time.Second, func() error {
		observed = n.setting.Encoded(&execCfg.Settings.SV)
		if observed != expectedEncodedValue {
			return errNotReady
		}
		return nil
	})
	if err != nil {
		log.Warningf(
			params.ctx, "SET CLUSTER SETTING %q timed out waiting for value %q, observed %q",
			n.name, expectedEncodedValue, observed,
		)
	}
	if CaseSensitiveG.Get(&params.p.execCfg.Settings.SV) {
		tree.CaseSensitiveG = true
	} else {
		tree.CaseSensitiveG = false
	}
	return err
}

func (n *setClusterSettingNode) Next(_ runParams) (bool, error) { return false, nil }
func (n *setClusterSettingNode) Values() tree.Datums            { return nil }
func (n *setClusterSettingNode) Close(_ context.Context)        {}

func toSettingString(
	ctx context.Context,
	p *planner,
	st *cluster.Settings,
	name string,
	s settings.Setting,
	d, prev tree.Datum,
) (string, error) {
	switch setting := s.(type) {
	case *settings.StringSetting:
		if s, ok := d.(*tree.DString); ok {
			if err := setting.Validate(&st.SV, string(*s)); err != nil {
				return "", err
			}
			return string(*s), nil
		}
		return "", errors.Errorf("cannot use %s %T value for string setting", d.ResolvedType(), d)
	case *settings.StateMachineSetting:
		if s, ok := d.(*tree.DString); ok {
			dStr, ok := prev.(*tree.DString)
			if !ok {
				return "", errors.New("the existing value is not a string")
			}
			prevRawVal := []byte(string(*dStr))
			newBytes, _, err := setting.Validate(&st.SV, prevRawVal, (*string)(s))
			if err != nil {
				return "", err
			}
			return string(newBytes), nil
		}
		return "", errors.Errorf("cannot use %s %T value for string setting", d.ResolvedType(), d)
	case *settings.BoolSetting:
		if b, ok := d.(*tree.DBool); ok {
			return settings.EncodeBool(bool(*b)), nil
		}
		return "", errors.Errorf("cannot use %s %T value for bool setting", d.ResolvedType(), d)
	case *settings.IntSetting:
		if i, ok := d.(*tree.DInt); ok {
			if err := setting.Validate(int64(*i)); err != nil {
				return "", err
			}
			if err := p.checkPasswordSetting(name, int64(*i)); err != nil {
				return "", err
			}
			return settings.EncodeInt(int64(*i)), nil
		}
		return "", errors.Errorf("cannot use %s %T value for int setting", d.ResolvedType(), d)
	case *settings.FloatSetting:
		if f, ok := d.(*tree.DFloat); ok {
			if err := setting.Validate(float64(*f)); err != nil {
				return "", err
			}
			return settings.EncodeFloat(float64(*f)), nil
		}
		return "", errors.Errorf("cannot use %s %T value for float setting", d.ResolvedType(), d)
	case *settings.EnumSetting:
		if i, intOK := d.(*tree.DInt); intOK {
			v, ok := setting.ParseEnum(settings.EncodeInt(int64(*i)))
			if ok {
				return settings.EncodeInt(v), nil
			}
			return "", errors.Errorf("invalid integer value '%d' for enum setting", *i)
		} else if s, ok := d.(*tree.DString); ok {
			str := string(*s)
			v, ok := setting.ParseEnum(str)
			if ok {
				return settings.EncodeInt(v), nil
			}
			return "", errors.Errorf("invalid string value '%s' for enum setting", str)
		}
		return "", errors.Errorf("cannot use %s %T value for enum setting, must be int or string", d.ResolvedType(), d)
	case *settings.ByteSizeSetting:
		if s, ok := d.(*tree.DString); ok {
			bytes, err := humanizeutil.ParseBytes(string(*s))
			if err != nil {
				return "", err
			}
			if err := setting.Validate(bytes); err != nil {
				return "", err
			}
			return settings.EncodeInt(bytes), nil
		}
		return "", errors.Errorf("cannot use %s %T value for byte size setting", d.ResolvedType(), d)
	case *settings.DurationSetting:
		if f, ok := d.(*tree.DInterval); ok {
			if f.Duration.Months > 0 || f.Duration.Days > 0 {
				return "", errors.Errorf("cannot use day or month specifiers: %s", d.String())
			}
			d := time.Duration(f.Duration.Nanos()) * time.Nanosecond
			if err := setting.Validate(d); err != nil {
				return "", err
			}
			return settings.EncodeDuration(d), nil
		}
		return "", errors.Errorf("cannot use %s %T value for duration setting", d.ResolvedType(), d)
	case *settings.IsolationSetting:
		if s, stringOK := d.(*tree.DString); stringOK {
			v, ok := setting.ParseIsolation(string(*s))
			if ok {
				return v, nil
			}
			return "", errors.Errorf("invalid isolation value '%s' for Isolation setting", string(*s))
		}
		return "", errors.Errorf("cannot use %s %T value for isolation setting", d.ResolvedType(), d)
	default:
		return "", errors.Errorf("unsupported setting type %T", setting)
	}
}
