package controller

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr/funcr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
)

// The start of a reconcile is logged at debug level, once the object is read: at the
// default level it only added a line per reconcile, the errors carry their own context.
var _ = Describe("Reconcile start logging", func() {
	request := ctrl.Request{NamespacedName: types.NamespacedName{Name: "hv-object", Namespace: "default"}}

	logsAt := func(verbosity int, run func(ctx context.Context)) string {
		var logs strings.Builder

		logger := funcr.New(func(prefix, args string) { logs.WriteString(prefix + " " + args + "\n") }, funcr.Options{Verbosity: verbosity})
		run(ctrl.LoggerInto(context.TODO(), logger))

		return logs.String()
	}

	newClient := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().
			WithScheme(pausedTestScheme()).
			WithObjects(objs...).
			WithStatusSubresource(&infrav1.HarvesterMachine{}, &infrav1.HarvesterCluster{}).
			Build()
	}

	machine := func() *infrav1.HarvesterMachine {
		return &infrav1.HarvesterMachine{ObjectMeta: metav1.ObjectMeta{Name: "hv-object", Namespace: "default"}}
	}

	cluster := func() *infrav1.HarvesterCluster {
		return &infrav1.HarvesterCluster{ObjectMeta: metav1.ObjectMeta{Name: "hv-object", Namespace: "default"}}
	}

	It("does not log the start of a HarvesterMachine reconcile at the default level", func() {
		for _, objs := range [][]client.Object{nil, {machine()}} {
			cl := newClient(objs...)
			logs := logsAt(0, func(ctx context.Context) {
				_, _ = (&HarvesterMachineReconciler{Client: cl, Scheme: cl.Scheme()}).Reconcile(ctx, request)
			})

			Expect(logs).ToNot(ContainSubstring("Reconciling HarvesterMachine"))
		}
	})

	It("logs the start of a HarvesterMachine reconcile at debug level once the object is read", func() {
		cl := newClient(machine())
		logs := logsAt(1, func(ctx context.Context) {
			_, _ = (&HarvesterMachineReconciler{Client: cl, Scheme: cl.Scheme()}).Reconcile(ctx, request)
		})

		Expect(logs).To(ContainSubstring("Reconciling HarvesterMachine"))
	})

	It("does not log the start of a HarvesterCluster reconcile at the default level", func() {
		for _, objs := range [][]client.Object{nil, {cluster()}} {
			cl := newClient(objs...)
			logs := logsAt(0, func(ctx context.Context) {
				_, _ = (&HarvesterClusterReconciler{Client: cl, Scheme: cl.Scheme()}).Reconcile(ctx, request)
			})

			Expect(logs).ToNot(ContainSubstring("Reconciling HarvesterCluster"))
		}
	})

	It("logs the start of a HarvesterCluster reconcile at debug level once the object is read", func() {
		cl := newClient(cluster())
		logs := logsAt(1, func(ctx context.Context) {
			_, _ = (&HarvesterClusterReconciler{Client: cl, Scheme: cl.Scheme()}).Reconcile(ctx, request)
		})

		Expect(logs).To(ContainSubstring("Reconciling HarvesterCluster"))
	})
})
