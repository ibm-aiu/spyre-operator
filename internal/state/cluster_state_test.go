/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package state_test

import (
	"context"

	v1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	. "github.com/ibm-aiu/spyre-operator/internal/state"
	"go.uber.org/zap/zapcore"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ClusterState", func() {
	ctx := context.Background()

	DescribeTable("applyLogLevel", func(levels []string, expected []zapcore.Level) {
		clusterState, err := NewClusterState(ctx, StateClient)
		Expect(err).To(BeNil())
		for i := range levels {
			cp := &v1alpha1.SpyreClusterPolicy{Spec: v1alpha1.SpyreClusterPolicySpec{LogLevel: &levels[i]}}
			err := clusterState.ApplyLogLevel(ctx, cp)
			Expect(err).To(BeNil())
			Expect(clusterState.GetLogLevel()).Should(Equal(expected[i]))
		}
	},
		Entry("default to debug", []string{"debug"},
			[]zapcore.Level{zapcore.DebugLevel}),
		Entry("default to debug to info", []string{"debug", "info"},
			[]zapcore.Level{zapcore.DebugLevel, zapcore.InfoLevel}),
		Entry("default to debug to info to error", []string{"debug", "info", "error"},
			[]zapcore.Level{zapcore.DebugLevel, zapcore.InfoLevel, zapcore.ErrorLevel}),
		Entry("default to debug to info to debug", []string{"debug", "info", "debug"},
			[]zapcore.Level{zapcore.DebugLevel, zapcore.InfoLevel, zapcore.DebugLevel}),
	)
})
