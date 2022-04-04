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

package crdmanifests

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	appsapi "github.com/jijiechen/external-crd/pkg/apis/kcrd/v1alpha1"
)

func transformManifest(crdResource *appsapi.KubernetesCrd) (*unstructured.Unstructured, error) {
	result := &unstructured.Unstructured{}
	if err := json.Unmarshal(crdResource.Manifest.Raw, result); err != nil {
		return nil, errors.NewInternalError(err)
	}
	result.SetGeneration(crdResource.Generation)
	result.SetCreationTimestamp(crdResource.CreationTimestamp)
	result.SetResourceVersion(crdResource.ResourceVersion)
	result.SetUID(crdResource.UID)
	result.SetDeletionGracePeriodSeconds(crdResource.DeletionGracePeriodSeconds)
	result.SetDeletionTimestamp(crdResource.DeletionTimestamp)
	result.SetFinalizers(crdResource.Finalizers)

	annotations := result.GetAnnotations()
	//if val, ok := crdResource.Annotations[known.FeedProtectionAnnotation]; ok {
	//	if annotations == nil {
	//		annotations = map[string]string{}
	//	}
	//	annotations[known.FeedProtectionAnnotation] = val
	//}
	result.SetAnnotations(annotations)

	return result, nil
}
