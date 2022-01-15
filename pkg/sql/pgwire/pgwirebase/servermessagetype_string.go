// Code generated by "stringer -type=ServerMessageType"; DO NOT EDIT.

package pgwirebase

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[ServerMsgAuth-82]
	_ = x[ServerMsgBindComplete-50]
	_ = x[ServerMsgCommandComplete-67]
	_ = x[ServerMsgCloseComplete-51]
	_ = x[ServerMsgCopyInResponse-71]
	_ = x[ServerMsgDataRow-68]
	_ = x[ServerMsgEmptyQuery-73]
	_ = x[ServerMsgErrorResponse-69]
	_ = x[ServerMsgNoticeResponse-78]
	_ = x[ServerMsgNoData-110]
	_ = x[ServerMsgParameterDescription-116]
	_ = x[ServerMsgParameterStatus-83]
	_ = x[ServerMsgParseComplete-49]
	_ = x[ServerMsgReady-90]
	_ = x[ServerMsgRowDescription-84]
}

const (
	_ServerMessageType_name_0 = "ServerMsgParseCompleteServerMsgBindCompleteServerMsgCloseComplete"
	_ServerMessageType_name_1 = "ServerMsgCommandCompleteServerMsgDataRowServerMsgErrorResponse"
	_ServerMessageType_name_2 = "ServerMsgCopyInResponse"
	_ServerMessageType_name_3 = "ServerMsgEmptyQuery"
	_ServerMessageType_name_4 = "ServerMsgNoticeResponse"
	_ServerMessageType_name_5 = "ServerMsgAuthServerMsgParameterStatusServerMsgRowDescription"
	_ServerMessageType_name_6 = "ServerMsgReady"
	_ServerMessageType_name_7 = "ServerMsgNoData"
	_ServerMessageType_name_8 = "ServerMsgPortalSuspendedServerMsgParameterDescription"
)

var (
	_ServerMessageType_index_0 = [...]uint8{0, 22, 43, 65}
	_ServerMessageType_index_1 = [...]uint8{0, 24, 40, 62}
	_ServerMessageType_index_5 = [...]uint8{0, 13, 37, 60}
	_ServerMessageType_index_8 = [...]uint8{0, 24, 53}
)

func (i ServerMessageType) String() string {
	switch {
	case 49 <= i && i <= 51:
		i -= 49
		return _ServerMessageType_name_0[_ServerMessageType_index_0[i]:_ServerMessageType_index_0[i+1]]
	case 67 <= i && i <= 69:
		i -= 67
		return _ServerMessageType_name_1[_ServerMessageType_index_1[i]:_ServerMessageType_index_1[i+1]]
	case i == 71:
		return _ServerMessageType_name_2
	case i == 73:
		return _ServerMessageType_name_3
	case i == 78:
		return _ServerMessageType_name_4
	case 82 <= i && i <= 84:
		i -= 82
		return _ServerMessageType_name_5[_ServerMessageType_index_5[i]:_ServerMessageType_index_5[i+1]]
	case i == 90:
		return _ServerMessageType_name_6
	case i == 110:
		return _ServerMessageType_name_7
	case 115 <= i && i <= 116:
		i -= 115
		return _ServerMessageType_name_8[_ServerMessageType_index_8[i]:_ServerMessageType_index_8[i+1]]
	default:
		return "ServerMessageType(" + strconv.FormatInt(int64(i), 10) + ")"
	}
}
