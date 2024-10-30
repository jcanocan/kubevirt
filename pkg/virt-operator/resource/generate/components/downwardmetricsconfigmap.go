/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright The KubeVirt Authors.
 *
 */

package components

import (
	"fmt"

	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const DownwardMetricsAllowedListConfigMap = "downward-metrics-allowed-list"

func NewDownwardMetricsConfigMap(operatorNamespace string) []*k8sv1.ConfigMap {
	ssc := "system:serviceaccount:%s:%s"
	return []*k8sv1.ConfigMap{
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      DownwardMetricsAllowedListConfigMap,
				Namespace: operatorNamespace,
			},
			Data: map[string]string{
				"allowedServiceAccounts": fmt.Sprintf(ssc+"\n"+ssc+"\n"+ssc, operatorNamespace, ControllerServiceAccountName,
					operatorNamespace, ApiServiceAccountName, operatorNamespace, HandlerServiceAccountName),
			},
		},
	}
}
