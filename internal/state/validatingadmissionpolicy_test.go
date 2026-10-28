/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package state_test

import (
	"path/filepath"

	"github.com/google/cel-go/cel"
	. "github.com/ibm-aiu/spyre-operator/internal/state"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apiserver/pkg/cel/environment"
)

// These tests evaluate the CEL in 0530_validatingadmissionpolicy.yaml against
// sample objects, without a cluster. The Kubernetes base CEL environment is
// used, but `object` is declared as dyn, so type checking is looser than in
// the real admission plugin.
var _ = Describe("block-restricted-device-classes policy", func() {
	var (
		env    *cel.Env
		policy *admissionv1.ValidatingAdmissionPolicy
	)

	BeforeEach(func() {
		objPath := filepath.Join(AssetsPath, "state-core-components", "dra-driver-spyre",
			"0530_validatingadmissionpolicy.yaml")
		runtimeObj, _, err := DecodeFromFile(StateScheme, objPath)
		Expect(err).To(BeNil())
		var ok bool
		policy, ok = runtimeObj.(*admissionv1.ValidatingAdmissionPolicy)
		Expect(ok).To(BeTrue())

		base := environment.MustBaseEnvSet(environment.DefaultCompatibilityVersion())
		env, err = base.Env(environment.StoredExpressions)
		Expect(err).To(BeNil())
		env, err = env.Extend(
			cel.Variable("object", cel.DynType),
		)
		Expect(err).To(BeNil())
	})

	eval := func(expression string, vars map[string]any) any {
		ast, issues := env.Compile(expression)
		Expect(issues.Err()).To(BeNil(), expression)
		prg, err := env.Program(ast)
		Expect(err).To(BeNil())
		out, _, err := prg.Eval(vars)
		Expect(err).To(BeNil(), expression)
		return out.Value()
	}

	// allowed returns whether every validation of the policy passes for obj.
	allowed := func(obj map[string]any) bool {
		vars := map[string]any{"object": obj}
		for _, validation := range policy.Spec.Validations {
			ok, isBool := eval(validation.Expression, vars).(bool)
			Expect(isBool).To(BeTrue())
			if !ok {
				return false
			}
		}
		return true
	}

	claim := func(requests ...any) map[string]any {
		return map[string]any{
			"kind": "ResourceClaim",
			"spec": map[string]any{"devices": map[string]any{"requests": requests}},
		}
	}
	template := func(requests ...any) map[string]any {
		return map[string]any{
			"kind": "ResourceClaimTemplate",
			"spec": map[string]any{"spec": map[string]any{"devices": map[string]any{"requests": requests}}},
		}
	}
	// request shapes: v1beta1 (flat), v1 (exactly), v1 (firstAvailable)
	flat := func(class string) any { return map[string]any{"name": "r", "deviceClassName": class} }
	exactly := func(class string) any {
		return map[string]any{"name": "r", "exactly": map[string]any{"deviceClassName": class}}
	}
	firstAvailable := func(classes ...string) any {
		subs := make([]any, 0, len(classes))
		for _, c := range classes {
			subs = append(subs, map[string]any{"name": "s", "deviceClassName": c})
		}
		return map[string]any{"name": "r", "firstAvailable": subs}
	}

	DescribeTable("evaluates ResourceClaim and ResourceClaimTemplate",
		func(obj map[string]any, expected bool) {
			Expect(allowed(obj)).To(Equal(expected))
		},
		// allowed
		Entry("claim: regular class (flat)", claim(flat("spyre-pf")), true),
		Entry("claim: regular class (exactly)", claim(exactly("spyre-vf")), true),
		Entry("claim: regular classes (firstAvailable)", claim(firstAvailable("spyre-pf", "spyre-vf")), true),
		Entry("claim: no devices", map[string]any{"kind": "ResourceClaim", "spec": map[string]any{}}, true),
		Entry("claim: no requests",
			map[string]any{"kind": "ResourceClaim", "spec": map[string]any{"devices": map[string]any{}}}, true),
		Entry("template: regular class (exactly)", template(exactly("spyre-pf")), true),
		Entry("template: no devices",
			map[string]any{"kind": "ResourceClaimTemplate", "spec": map[string]any{"spec": map[string]any{}}}, true),
		// denied
		Entry("claim: managed-pf (flat)", claim(flat("spyre-managed-pf")), false),
		Entry("claim: managed-vf (exactly)", claim(exactly("spyre-managed-vf")), false),
		Entry("claim: managed in firstAvailable (2nd)",
			claim(firstAvailable("spyre-pf", "spyre-managed-pf")), false),
		Entry("claim: managed among several requests",
			claim(exactly("spyre-pf"), exactly("spyre-managed-vf")), false),
		Entry("template: managed-pf (flat)", template(flat("spyre-managed-pf")), false),
		Entry("template: managed-vf (exactly)", template(exactly("spyre-managed-vf")), false),
		Entry("template: managed in firstAvailable",
			template(firstAvailable("spyre-vf", "spyre-managed-vf")), false),
	)
})
