/*
Copyright 2021 The Clusternet Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package wrappers

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	overlayapi "github.com/jijiechen/external-crd/pkg/apis/overlay/v1alpha1"
)

var (
	// regex matches '/api/v1/{path}'
	apiv1Regex = regexp.MustCompile(`^(/api/v1)/(.*)`)
	// regex matches "/apis/{group}/{version}/{path}"
	apisRegex = regexp.MustCompile(`^(/apis/.*/v\w*.)/(.*)`)
	// regex matches "/apis/xxx.clusternet.io/{version}/{path}"
	kcrdAPIsRegex = regexp.MustCompile(`^(/apis/\w*\.jijiechen\.com/v\w*.)/(.*)`)

	// overlayGV represents current overlay SchemeGroupVersion
	overlayGV = overlayapi.SchemeGroupVersion.String()
)

// ExternalCrdTransport is a transport to redirect requests to clusternet-hub
type ExternalCrdTransport struct {
	// relative paths may omit leading slash
	path string

	rt http.RoundTripper
}

func (t *ExternalCrdTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.normalizeLocation(req.URL)
	return t.rt.RoundTrip(req)
}

// normalizeLocation format the request URL to Clusternet overlay GVKs
func (t *ExternalCrdTransport) normalizeLocation(location *url.URL) {
	curPath := location.Path
	// Trim returns a slice of the string s with all leading and trailing Unicode code points contained in cutset removed.
	// so we use Replace here
	reqPath := strings.Replace(curPath, t.path, "", 1)
	if apiv1Regex.MatchString(reqPath) {
		location.Path = path.Join(t.path, apiv1Regex.ReplaceAllString(reqPath, fmt.Sprintf("/apis/%s/$2", overlayGV)))
	}
	// we don't normalize request for Group xxx.clusternet.io
	if kcrdAPIsRegex.MatchString(reqPath) {
		return
	}
	if apisRegex.MatchString(reqPath) {
		location.Path = path.Join(t.path, apisRegex.ReplaceAllString(reqPath, fmt.Sprintf("/apis/%s/$2", overlayGV)))
	}
}

func NewExternalCrdTransport(host string, rt http.RoundTripper) *ExternalCrdTransport {
	// host must be a host string, a host:port pair, or a URL to the base of the apiserver.
	// If a URL is given then the (optional) Path of that URL represents a prefix that must
	// be appended to all request URIs used to access the apiserver. This allows a frontend
	// proxy to easily relocate all of the apiserver endpoints.
	return &ExternalCrdTransport{
		path: urlMustParse(host).Path,
		rt:   rt,
	}
}

func urlMustParse(path string) *url.URL {
	location, err := url.Parse(strings.TrimRight(path, "/"))
	if err != nil {
		panic(err)
	}
	return location
}
