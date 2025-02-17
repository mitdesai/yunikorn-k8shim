/*
 Licensed to the Apache Software Foundation (ASF) under one
 or more contributor license agreements.  See the NOTICE file
 distributed with this work for additional information
 regarding copyright ownership.  The ASF licenses this file
 to you under the Apache License, Version 2.0 (the
 "License"); you may not use this file except in compliance
 with the License.  You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package test

import (
	v1 "k8s.io/api/core/v1"
	apis "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	listers "k8s.io/client-go/listers/core/v1"

	"github.com/apache/yunikorn-k8shim/pkg/common/constants"
)

type ConfigMapListerMock struct {
	configMaps []*v1.ConfigMap
}

func NewConfigMapListerMock() *ConfigMapListerMock {
	YKConfigmap := v1.ConfigMap{
		ObjectMeta: apis.ObjectMeta{
			Name:   constants.DefaultConfigMapName,
			Labels: map[string]string{"app": "yunikorn", "label2": "value2"},
		},
		Data: map[string]string{"queues.yaml": "OldData"},
	}
	return &ConfigMapListerMock{
		configMaps: []*v1.ConfigMap{&YKConfigmap},
	}
}

func (c ConfigMapListerMock) List(selector labels.Selector) (ret []*v1.ConfigMap, err error) {
	return c.configMaps, nil
}

func (c ConfigMapListerMock) ConfigMaps(namespace string) listers.ConfigMapNamespaceLister {
	return nil
}
