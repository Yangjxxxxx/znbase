// Code generated by "stringer -type=Option"; DO NOT EDIT.

package useroption

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[CREATEUSER-1]
	_ = x[NOCREATEUSER-2]
	_ = x[PASSWORD-3]
	_ = x[VALIDUNTIL-4]
	_ = x[ENCRYPTION-5]
	_ = x[CONNECTIONLIMIT-6]
	_ = x[CSTATUS-7]
	_ = x[NOCSTATUS-8]
}

const _Option_name = "CREATEUSERNOCREATEUSERPASSWORDVALIDUNTILENCRYPTIONCONNECTIONLIMITCSTATUSNOCSTATUS"

var _Option_index = [...]uint8{0, 10, 22, 30, 40, 50, 65, 72, 81}

func (i Option) String() string {
	i -= 1
	if i >= Option(len(_Option_index)-1) {
		return "Option(" + strconv.FormatInt(int64(i+1), 10) + ")"
	}
	return _Option_name[_Option_index[i]:_Option_index[i+1]]
}