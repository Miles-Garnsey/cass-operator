// Copyright DataStax, Inc.
// Please see the included license file for details.

package scale_up

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/k8ssandra/cass-operator/tests/kustomize"
	ginkgo_util "github.com/k8ssandra/cass-operator/tests/util/ginkgo"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

var (
	testName   = "Scale up"
	namespace  = "test-scale-up"
	dcName     = "dc2"
	dcYaml     = "../testdata/default-single-rack-single-node-dc.yaml"
	dcResource = fmt.Sprintf("CassandraDatacenter/%s", dcName)
	dcLabel    = fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)
	ns         = ginkgo_util.NewWrapper(testName, namespace)
)

func TestLifecycle(t *testing.T) {
	AfterSuite(func() {
		logPath := fmt.Sprintf("%s/aftersuite", ns.LogDir)
		kubectl.DumpAllLogs(logPath).ExecV()
		fmt.Printf("\n\tPost-run logs dumped at: %s\n\n", logPath)
		ns.Terminate()
		kustomize.Undeploy(namespace)
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, testName)
}

var _ = Describe(testName, func() {
	Context("when in a new cluster", func() {
		Specify("the operator can scale up a datacenter", func() {
			By("deploy cass-operator with kustomize")
			err := kustomize.Deploy(namespace)
			Expect(err).ToNot(HaveOccurred())

			ns.WaitForOperatorReady()

			step := "creating a datacenter resource with 1 rack/1 node"
			k := kubectl.ApplyFiles(dcYaml)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterReady(dcName)

			step = "scale up to 2 nodes"
			json := "{\"spec\": {\"size\": 2}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterCondition(dcName, "ScalingUp", string(corev1.ConditionTrue))
			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)
			ns.WaitForDatacenterCondition(dcName, "ScalingUp", string(corev1.ConditionFalse))

			// Ensure that when 'ScaleUp' becomes 'false' that our pods are in fact up and running
			Expect(len(ns.GetDatacenterReadyPodNames(dcName))).To(Equal(2))

			ns.WaitForDatacenterReady(dcName)

			// Ensure we have a single CassandraTask created which is a cleanup (and it succeeded)
			ns.CheckForCompletedCassandraTasks(dcName, "cleanup", 1)
			// ns.CheckForCompletedCassandraTask(dcName, "cleanup")

			step = "scale up to 4 nodes"
			json = "{\"spec\": {\"size\": 4}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)
			ns.WaitForDatacenterReady(dcName)

			// Ensure we have two CassandraTasks created which are cleanup (and they succeeded)
			ns.CheckForCompletedCassandraTasks(dcName, "cleanup", 2)

			step = "scale up to 5 nodes"
			json = "{\"spec\": {\"size\": 5}}"
			k = kubectl.PatchMerge(dcResource, json)
			ns.ExecAndLog(step, k)

			ns.WaitForDatacenterOperatorProgress(dcName, "Updating", 60)
			ns.WaitForDatacenterReady(dcName)

			// Ensure we have three CassandraTasks created which are cleanup (and they succeeded)
			ns.CheckForCompletedCassandraTasks(dcName, "cleanup", 3)

			step = "check recorded host IDs"
			nodeStatusesHostIds := ns.GetNodeStatusesHostIds(dcName)
			Expect(len(nodeStatusesHostIds), 5)

			step = "deleting the dc"
			k = kubectl.DeleteFromFiles(dcYaml)
			ns.ExecAndLog(step, k)

			step = "checking that the dc no longer exists"
			json = "jsonpath={.items}"
			k = kubectl.Get("CassandraDatacenter").
				WithLabel(dcLabel).
				FormatOutput(json)
			ns.WaitForOutputAndLog(step, k, "[]", 300)
		})
	})
})
