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

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/pointer"
)

const (
	validatingAdmissionPolicyBindingName                = "kubevirt-node-restriction-binding"
	validatingAdmissionPolicyName                       = "kubevirt-node-restriction-policy"
	nodeRestrictionAppLabelValue                        = "kubevirt-node-restriction"
	downwardMetricsValidatingAdmissionPolicyBindingName = "downward-metrics-binding"
	downwardMetricsValidatingAdmissionPolicyName        = "downward-metrics-policy"

	NodeRestrictionErrModifySpec           = "this user cannot modify spec of node"
	NodeRestrictionErrChangeMetadataFields = "this user can only change allowed metadata fields"
	NodeRestrictionErrAddDeleteLabels      = "this user cannot add/delete non kubevirt-owned labels"
	NodeRestrictionErrUpdateLabels         = "this user cannot update non kubevirt-owned labels"
	NodeRestrictionErrAddDeleteAnnotations = "this user cannot add/delete non kubevirt-owned annotations"
	NodeRestrictionErrUpdateAnnotations    = "this user cannot update non kubevirt-owned annotations"
	DownwardMetricsErr                     = "this user, namespace or service account cannot create or update VM/VMIs using downwardMetrics"
)

func NewHandlerV1ValidatingAdmissionPolicyBinding() *admissionregistrationv1.ValidatingAdmissionPolicyBinding {
	return &admissionregistrationv1.ValidatingAdmissionPolicyBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ValidatingAdmissionPolicyBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: validatingAdmissionPolicyBindingName,
			Labels: map[string]string{
				v1.AppLabel:       nodeRestrictionAppLabelValue,
				v1.ManagedByLabel: v1.ManagedByLabelOperatorValue,
			},
		},
		Spec: admissionregistrationv1.ValidatingAdmissionPolicyBindingSpec{
			PolicyName: validatingAdmissionPolicyName,
			ValidationActions: []admissionregistrationv1.ValidationAction{
				admissionregistrationv1.Deny,
			},
			MatchResources: &admissionregistrationv1.MatchResources{
				ResourceRules: []admissionregistrationv1.NamedRuleWithOperations{
					{
						RuleWithOperations: admissionregistrationv1.RuleWithOperations{
							Operations: []admissionregistrationv1.OperationType{
								admissionregistrationv1.OperationAll,
							},
							Rule: admissionregistrationv1.Rule{
								APIGroups:   []string{""},
								APIVersions: []string{"*"},
								Resources:   []string{"nodes"},
							},
						},
					},
				},
			},
		},
	}
}

func NewHandlerV1ValidatingAdmissionPolicy(virtHandlerServiceAccount string) *admissionregistrationv1.ValidatingAdmissionPolicy {
	return &admissionregistrationv1.ValidatingAdmissionPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ValidatingAdmissionPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: validatingAdmissionPolicyName,
		},
		Spec: admissionregistrationv1.ValidatingAdmissionPolicySpec{
			FailurePolicy: pointer.P(admissionregistrationv1.Fail),
			MatchConstraints: &admissionregistrationv1.MatchResources{
				ResourceRules: []admissionregistrationv1.NamedRuleWithOperations{
					{
						RuleWithOperations: admissionregistrationv1.RuleWithOperations{
							Operations: []admissionregistrationv1.OperationType{
								admissionregistrationv1.Update,
							},
							Rule: admissionregistrationv1.Rule{
								APIGroups:   []string{""},
								APIVersions: []string{"*"},
								Resources:   []string{"nodes"},
							},
						},
					},
				},
			},
			MatchConditions: []admissionregistrationv1.MatchCondition{
				{
					Name:       "virt-handler-user-only",
					Expression: fmt.Sprintf("request.userInfo.username == %q", virtHandlerServiceAccount),
				},
			},
			Variables: []admissionregistrationv1.Variable{
				{
					Name:       "oldNonKubevirtLabels",
					Expression: `oldObject.metadata.labels.filter(k, !k.contains("kubevirt.io") && k != "cpumanager")`,
				},
				{
					Name:       "oldLabels",
					Expression: "oldObject.metadata.labels",
				},
				{
					Name:       "newNonKubevirtLabels",
					Expression: `object.metadata.labels.filter(k, !k.contains("kubevirt.io") && k != "cpumanager")`,
				},
				{
					Name:       "newLabels",
					Expression: "object.metadata.labels",
				},
				{
					Name:       "oldNonKubevirtAnnotations",
					Expression: `oldObject.metadata.annotations.filter(k, !k.contains("kubevirt.io"))`,
				},
				{
					Name:       "newNonKubevirtAnnotations",
					Expression: `object.metadata.annotations.filter(k, !k.contains("kubevirt.io"))`,
				},
				{
					Name:       "oldAnnotations",
					Expression: "oldObject.metadata.annotations",
				},
				{
					Name:       "newAnnotations",
					Expression: "object.metadata.annotations",
				},
			},
			Validations: []admissionregistrationv1.Validation{
				{
					Expression: "object.spec == oldObject.spec",
					Message:    NodeRestrictionErrModifySpec,
				},
				{
					Expression: `oldObject.metadata.filter(k, k != "labels" && k != "annotations" && k != "managedFields" && k != "resourceVersion").all(k, k in object.metadata) && object.metadata.filter(k, k != "labels" && k != "annotations" && k != "managedFields" && k != "resourceVersion").all(k, k in oldObject.metadata && oldObject.metadata[k] == object.metadata[k])`,
					Message:    NodeRestrictionErrChangeMetadataFields,
				},
				{
					Expression: `size(variables.newNonKubevirtLabels) == size(variables.oldNonKubevirtLabels)`,
					Message:    NodeRestrictionErrAddDeleteLabels,
				},
				{
					Expression: `variables.newNonKubevirtLabels.all(k, k in variables.oldNonKubevirtLabels && variables.newLabels[k] == variables.oldLabels[k])`,
					Message:    NodeRestrictionErrUpdateLabels,
				},
				{
					Expression: `size(variables.newNonKubevirtAnnotations) == size(variables.oldNonKubevirtAnnotations)`,
					Message:    NodeRestrictionErrAddDeleteAnnotations,
				},
				{
					Expression: `variables.newNonKubevirtAnnotations.all(k, k in variables.oldNonKubevirtAnnotations && variables.newAnnotations[k] == variables.oldAnnotations[k])`,
					Message:    NodeRestrictionErrUpdateAnnotations,
				},
			},
		},
	}
}

func NewDownwardMetricsValidatingAdmissionPolicy() *admissionregistrationv1.ValidatingAdmissionPolicy {
	// TODO: Filter "" users, ns or SCC.
	return &admissionregistrationv1.ValidatingAdmissionPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ValidatingAdmissionPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: downwardMetricsValidatingAdmissionPolicyName,
		},
		Spec: admissionregistrationv1.ValidatingAdmissionPolicySpec{
			FailurePolicy: pointer.P(admissionregistrationv1.Fail),
			ParamKind: &admissionregistrationv1.ParamKind{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
			MatchConstraints: &admissionregistrationv1.MatchResources{
				ResourceRules: []admissionregistrationv1.NamedRuleWithOperations{
					{
						RuleWithOperations: admissionregistrationv1.RuleWithOperations{
							Operations: []admissionregistrationv1.OperationType{
								admissionregistrationv1.Update,
								admissionregistrationv1.Create,
							},
							Rule: admissionregistrationv1.Rule{
								APIGroups:   []string{"kubevirt.io"},
								APIVersions: []string{"*"},
								Resources:   []string{"virtualmachineinstances", "virtualmachines"},
							},
						},
					},
				},
			},
			// TODO: include downward metrics disk detection
			MatchConditions: []admissionregistrationv1.MatchCondition{
				{
					Name:       "downward-metrics-vms-only",
					Expression: "has(object.spec) && (has(object.spec.domain.devices.downwardMetrics) || size(object.spec.volumes.filter(k, has(k.downwardMetrics))) != 0)\n || (has(object.spec.template) && has(object.spec.template.spec.domain.devices.downwardMetrics) || size(object.spec.template.spec.volumes.filter(k, has(k.downwardMetrics))) != 0)",
				},
			},
			Variables: []admissionregistrationv1.Variable{
				{
					Name:       "user",
					Expression: `string(request.userInfo.username)`,
				},
				{
					Name:       "requestNamespace",
					Expression: `string(request.namespace)`,
				},
				{
					Name:       "allowedUsers",
					Expression: `has(params.data.allowedUsers) && variables.user in (params.data.allowedUsers.split('\n'))`,
				},
				{
					Name:       "allowedServiceAccounts",
					Expression: `has(params.data.allowedServiceAccounts) && variables.user in params.data.allowedServiceAccounts.split('\n')`,
				},
				{
					Name:       "allowedNamespaces",
					Expression: `has(params.data.allowedNamespaces) && variables.requestNamespace in params.data.allowedNamespaces.split('\n')`,
				},
			},
			Validations: []admissionregistrationv1.Validation{
				{
					Expression: "variables.allowedUsers || variables.allowedServiceAccounts || variables.allowedNamespaces",
					Message:    DownwardMetricsErr,
				},
			},
		},
	}
}

func NewDownwardMetricsValidationAdmissionPolicyBinding(installNamespace string) *admissionregistrationv1.ValidatingAdmissionPolicyBinding {
	return &admissionregistrationv1.ValidatingAdmissionPolicyBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ValidatingAdmissionPolicyBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: downwardMetricsValidatingAdmissionPolicyBindingName,
		},
		Spec: admissionregistrationv1.ValidatingAdmissionPolicyBindingSpec{
			PolicyName: downwardMetricsValidatingAdmissionPolicyName,
			ValidationActions: []admissionregistrationv1.ValidationAction{
				admissionregistrationv1.Deny,
			},
			ParamRef: &admissionregistrationv1.ParamRef{
				Name:                    DownwardMetricsAllowedListConfigMap,
				Namespace:               installNamespace,
				ParameterNotFoundAction: pointer.P(admissionregistrationv1.DenyAction),
			},
		},
	}
}
