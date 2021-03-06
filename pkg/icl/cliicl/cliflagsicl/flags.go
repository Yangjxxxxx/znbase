// Copyright 2017 The Cockroach Authors.
//
// Licensed as a Cockroach Enterprise file under the ZNBase Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/znbasedb/znbase/blob/master/licenses/ICL.txt

package cliflagsicl

import "github.com/znbasedb/znbase/pkg/cli/cliflags"

// Attrs and others store the static information for CLI flags.
var (
	EnterpriseEncryption = cliflags.FlagInfo{
		Name: "enterprise-encryption",
		Description: `
<PRE>
*** Valid enterprise licenses only ***

WARNING: encryption at rest is an experimental feature.

Enable encryption at rest for a store.

TODO(mberhault): fill in description.

</PRE>
Key files must be of size 32 bytes + AES key size, such as:
<PRE>
AES-128: 48 bytes
AES-192: 56 bytes
AES-256: 64 bytes

</PRE>
Valid fields:
<PRE>
* path    (required): must match the path of one of the stores
* key     (required): path to the current key file, or "plain"
* old-key (required): path to the previous key file, or "plain"
* rotation-period   : amount of time after which data keys should be rotated

</PRE>
example:
<PRE>
  --enterprise-encryption=path=znbase-data,key=/keys/aes-128.key,old-key=plain
</PRE>
`,
	}
)
