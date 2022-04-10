/*
Copyright 2022 Jijie Chen.
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

package apiserver

import (
	kcrd "github.com/jijiechen/external-crd/pkg/apis/kcrd/v1alpha1"
	"github.com/jijiechen/external-crd/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"strings"
	"testing"
)

func TestRESTGenerateNameForManifest(t *testing.T) {
	tests := []struct {
		testCaseName string
		resourceName string
		namespace    string
		namespaced   bool
		name         string
		want         string
	}{
		{
			testCaseName: "namespace-scoped resources foos (standard)",
			resourceName: "foos",
			namespace:    "kube-system",
			namespaced:   true,
			name:         "abc",
			want:         "foos.kube-system.abc",
		},
		{
			testCaseName: "namespace-scoped resources foos (name with '.' & '-')",
			resourceName: "foos",
			namespace:    "kube-system",
			namespaced:   true,
			name:         "abc.def-bar",
			want:         "foos.kube-system.abc.def-bar",
		},

		{
			testCaseName: "cluster-scoped resources bars (standard)",
			resourceName: "bars",
			namespace:    "kube-system",
			namespaced:   false,
			name:         "abc",
			want:         "bars.abc",
		},
		{
			testCaseName: "cluster-scoped resources bars (name with '.' & '-')",
			resourceName: "bars",
			namespace:    "kube-system",
			namespaced:   false,
			name:         "abc.def-bar",
			want:         "bars.abcd.abc.def-bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.testCaseName, func(t *testing.T) {
			r := &REST{
				name:       tt.resourceName,
				namespaced: tt.namespaced,
			}
			if got := r.getNormalizedManifestName("abcd", tt.namespace, tt.name); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTransformManifest(t *testing.T) {
	tests := []struct {
		name         string
		manifest     *kcrd.KubernetesCrd
		wantedString string
		wantErr      bool
	}{
		{
			manifest: &kcrd.KubernetesCrd{
				TypeMeta: metav1.TypeMeta{
					APIVersion: kcrd.SchemeGroupVersion.String(),
					Kind:       kcrd.Kind("Manifest").String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Generation: 6,
					Labels: map[string]string{
						utils.ConfigGroupLabel:     "foo",
						utils.ConfigVersionLabel:   "v1alpha1",
						utils.ConfigKindLabel:      "Bar",
						utils.ConfigNameLabel:      "boo",
						utils.ConfigNamespaceLabel: "ns1",
					},
					Namespace:       utils.KcrdReservedNamespace,
					Name:            "bars-boo",
					ResourceVersion: "1860247",
					UID:             "13ff776c-1e91-4a84-b77d-6c35f3a52fed",
				},
				Manifest: runtime.RawExtension{
					Raw: []byte(`
{
  "apiVersion": "v1",
  "kind": "Namespace",
  "metadata": {
    "labels": {
      "clusternet.io/created-by": "clusternet-hub",
      "key": "valxyz",
      "kubernetes.io/metadata.name": "abc"
    },
    "name": "abc",
    "uid": "01b17d4e-18cf-441d-8ee6-1c484b4afbe2"
  },
  "spec": {
    "finalizers": [
      "kubernetes"
    ]
  },
  "status": {
    "phase": "Active"
  }
}
`),
				},
			},
			wantedString: `{"apiVersion":"v1","kind":"Namespace","metadata":{"generation":6,"labels":{"clusternet.io/created-by":"clusternet-hub","key":"valxyz","kubernetes.io/metadata.name":"abc"},"name":"abc","resourceVersion":"1860247","uid":"13ff776c-1e91-4a84-b77d-6c35f3a52fed"},"spec":{"finalizers":["kubernetes"]},"status":{"phase":"Active"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := transformManifest(tt.manifest)
			if (err != nil) != tt.wantErr {
				t.Errorf("transformManifest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			gotString, err := got.MarshalJSON()
			if err != nil {
				t.Errorf("failed to marshal: %v", err)
				return
			}
			if strings.TrimRight(string(gotString), "\n") != tt.wantedString {
				t.Errorf("transformManifest() \ngot = \n%q\n, want = \n%q", gotString, tt.wantedString)
			}
		})
	}
}
