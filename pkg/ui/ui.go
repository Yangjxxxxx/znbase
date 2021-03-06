// Copyright 2015  The Cockroach Authors.
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

// Package ui embeds the assets for the web UI into the ZNBase binary.
//
// By default, it serves a stub web UI. Linking with distoss or disticl will
// replace the stubs with the OSS UI or the CCL UI, respectively. The exported
// symbols in this package are thus function pointers instead of functions so
// that they can be mutated by init hooks.
package ui

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/http"
	"os"

	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/base"
	"github.com/znbasedb/znbase/pkg/build"
	"github.com/znbasedb/znbase/pkg/util/log"
)

// Asset loads and returns the asset for the given name. It returns an error if
// the asset could not be found or could not be loaded.
var Asset func(name string) ([]byte, error)

// AssetDir returns the file names below a certain directory in the embedded
// filesystem.
//
// For example, if the embedded filesystem contains the following hierarchy:
//
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
//
// AssetDir("") returns []string{"data"}
// AssetDir("data") returns []string{"foo.txt", "img"}
// AssetDir("data/img") returns []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") return errors
var AssetDir func(name string) ([]string, error)

// AssetInfo loads and returns metadata for the asset with the given name. It
// returns an error if the asset could not be found or could not be loaded.
var AssetInfo func(name string) (os.FileInfo, error)

// haveUI returns whether the admin UI has been linked into the binary.
func haveUI() bool {
	return Asset != nil && AssetDir != nil && AssetInfo != nil
}

// indexTemplate takes arguments about the current session and returns HTML
// which includes the UI JavaScript bundles, plus a script tag which sets the
// currently logged in user so that the UI JavaScript can decide whether to show
// a login page.
var indexHTMLTemplate = template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html>
	<head>
		<title>ZNBase Console</title>
		<meta charset="UTF-8">
		<link href="favicon.ico" rel="shortcut icon">
	</head>
	<body>
		<div id="react-layout"></div>

		<script>
			window.dataFromServer = {{.}};
		</script>

		<script src="protos.dll.js" type="text/javascript"></script>
		<script src="vendor.dll.js" type="text/javascript"></script>
		<script src="bundle.js" type="text/javascript"></script>
	</body>
</html>
`))

type indexHTMLArgs struct {
	ExperimentalUseLogin bool
	LoginEnabled         bool
	LoggedInUser         *string
	Tag                  string
	Version              string
	NodeID               string
}

// bareIndexHTML is used in place of indexHTMLTemplate when the binary is built
// without the web UI.
var bareIndexHTML = []byte(fmt.Sprintf(`<!DOCTYPE html>
<title>ZNBaseDB</title>
Binary built without web UI.
<hr>
<em>%s</em>`, build.GetInfo().Short()))

// Config contains the configuration parameters for Handler.
type Config struct {
	ExperimentalUseLogin bool
	LoginEnabled         bool
	NodeID               *base.NodeIDContainer
	GetUser              func(ctx context.Context) *string
}

// Handler returns an http.Handler that serves the UI,
// including index.html, which has some login-related variables
// templated into it, as well as static assets.
func Handler(cfg Config) http.Handler {
	fileServer := http.FileServer(&assetfs.AssetFS{
		Asset:     Asset,
		AssetDir:  AssetDir,
		AssetInfo: AssetInfo,
	})
	buildInfo := build.GetInfo()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !haveUI() {
			http.ServeContent(w, r, "index.html", buildInfo.GoTime(), bytes.NewReader(bareIndexHTML))
			return
		}

		if r.URL.Path != "/" {
			fileServer.ServeHTTP(w, r)
			return
		}

		if err := indexHTMLTemplate.Execute(w, indexHTMLArgs{
			ExperimentalUseLogin: cfg.ExperimentalUseLogin,
			LoginEnabled:         cfg.LoginEnabled,
			LoggedInUser:         cfg.GetUser(r.Context()),
			Tag:                  buildInfo.Tag,
			Version:              build.VersionPrefix(),
			NodeID:               cfg.NodeID.String(),
		}); err != nil {
			err = errors.Wrap(err, "templating index.html")
			http.Error(w, err.Error(), 500)
			log.Error(r.Context(), err)
		}
	})
}
