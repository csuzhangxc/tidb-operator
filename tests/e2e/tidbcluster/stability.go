// Copyright 2019 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package tidbcluster

/*
import (
	"context"
	"fmt"
	_ "net/http/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	asclientset "github.com/pingcap/advanced-statefulset/client/client/clientset/versioned"
	"github.com/pingcap/errors"
	v1 "k8s.io/api/core/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	corelisterv1 "k8s.io/client-go/listers/core/v1"
	restclient "k8s.io/client-go/rest"
	aggregatorclient "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
	storageutils "k8s.io/kubernetes/test/e2e/storage/utils"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pingcap/tidb-operator/pkg/apis/label"
	"github.com/pingcap/tidb-operator/pkg/apis/pingcap/v1alpha1"
	"github.com/pingcap/tidb-operator/pkg/client/clientset/versioned"
	"github.com/pingcap/tidb-operator/pkg/controller"
	"github.com/pingcap/tidb-operator/pkg/scheme"
	"github.com/pingcap/tidb-operator/pkg/third_party/k8s"
	"github.com/pingcap/tidb-operator/pkg/util"
	"github.com/pingcap/tidb-operator/tests"
	e2econfig "github.com/pingcap/tidb-operator/tests/e2e/config"
	e2eframework "github.com/pingcap/tidb-operator/tests/e2e/framework"
	testutils "github.com/pingcap/tidb-operator/tests/e2e/util"
	utilcloud "github.com/pingcap/tidb-operator/tests/e2e/util/cloud"
	utilimage "github.com/pingcap/tidb-operator/tests/e2e/util/image"
	utilpod "github.com/pingcap/tidb-operator/tests/e2e/util/pod"
	"github.com/pingcap/tidb-operator/tests/e2e/util/portforward"
	"github.com/pingcap/tidb-operator/tests/e2e/util/proxiedpdclient"
	utiltidb "github.com/pingcap/tidb-operator/tests/e2e/util/tidb"
	utiltc "github.com/pingcap/tidb-operator/tests/e2e/util/tidbcluster"
	utiltidbcluster "github.com/pingcap/tidb-operator/tests/e2e/util/tidbcluster"
	utiltikv "github.com/pingcap/tidb-operator/tests/e2e/util/tikv"
	"github.com/pingcap/tidb-operator/tests/pkg/fixture"
	"github.com/pingcap/tidb-operator/tests/pkg/mock"
	framework "github.com/pingcap/tidb-operator/tests/third_party/k8s"
	"github.com/pingcap/tidb-operator/tests/third_party/k8s/log"
	e2enode "github.com/pingcap/tidb-operator/tests/third_party/k8s/node"
	"github.com/pingcap/tidb-operator/tests/third_party/k8s/pod"
	e2eskipper "github.com/pingcap/tidb-operator/tests/third_party/k8s/skipper"
)

// Stability specs describe tests which involve disruptive operations, e.g.
// stop kubelet, kill nodes, empty pd/tikv data.
// Like serial tests, they cannot run in parallel too.
var _ = ginkgo.Describe("[Stability]", func() {
	f := e2eframework.NewDefaultFramework("stability")

	var ns string
	var c clientset.Interface
	var cli versioned.Interface
	var asCli asclientset.Interface
	var aggrCli aggregatorclient.Interface
	var apiExtCli apiextensionsclientset.Interface
	var oa *tests.OperatorActions
	var cfg *tests.Config
	var config *restclient.Config
	var ocfg *tests.OperatorConfig
	var fw portforward.PortForward
	var fwCancel context.CancelFunc
	var secretLister corelisterv1.SecretLister

	ginkgo.BeforeEach(func() {
		ns = f.Namespace.Name
		c = f.ClientSet
		secretLister = tests.GetSecretListerWithCacheSynced(c, 10*time.Second)

		var err error
		config, err = framework.LoadConfig()
		framework.ExpectNoError(err, "failed to load config")
		cli, err = versioned.NewForConfig(config)
		framework.ExpectNoError(err, "failed to create clientset")
		asCli, err = asclientset.NewForConfig(config)
		framework.ExpectNoError(err, "failed to create clientset")
		aggrCli, err = aggregatorclient.NewForConfig(config)
		framework.ExpectNoError(err, "failed to create clientset")
		apiExtCli, err = apiextensionsclientset.NewForConfig(config)
		framework.ExpectNoError(err, "failed to create clientset")
		clientRawConfig, err := e2econfig.LoadClientRawConfig()
		framework.ExpectNoError(err, "failed to load raw config")
		ctx, cancel := context.WithCancel(context.Background())
		fw, err = portforward.NewPortForwarder(ctx, e2econfig.NewSimpleRESTClientGetter(clientRawConfig))
		framework.ExpectNoError(err, "failed to create port forwarder")
		fwCancel = cancel
		cfg = e2econfig.TestConfig
		ocfg = e2econfig.NewDefaultOperatorConfig(cfg)
		oa = tests.NewOperatorActions(cli, c, asCli, aggrCli, apiExtCli, tests.DefaultPollInterval, ocfg, e2econfig.TestConfig, fw, f)
	})

	ginkgo.AfterEach(func() {
		if fwCancel != nil {
			fwCancel()
		}
	})

	ginkgo.Context("operator with default values", func() {
		var ocfg *tests.OperatorConfig
		var oa *tests.OperatorActions
		var genericCli client.Client

		ginkgo.BeforeEach(func() {
			ocfg = &tests.OperatorConfig{
				Namespace:   ns,
				ReleaseName: "operator",
				Image:       cfg.OperatorImage,
				Tag:         cfg.OperatorTag,
				LogLevel:    "4",
				TestMode:    true,
			}
			oa = tests.NewOperatorActions(cli, c, asCli, aggrCli, apiExtCli, tests.DefaultPollInterval, ocfg, e2econfig.TestConfig, fw, f)
			ginkgo.By("Installing CRDs")
			oa.CleanCRDOrDie()
			oa.CreateCRDOrDie(ocfg)
			ginkgo.By("Installing tidb-operator")
			oa.CleanOperatorOrDie(ocfg)
			oa.DeployOperatorOrDie(ocfg)
			var err error
			genericCli, err = client.New(config, client.Options{Scheme: scheme.Scheme})
			framework.ExpectNoError(err, "failed to create clientset")
		})

		testCases := []struct {
			name string
			fn   func()
		}{
			{
				name: "tidb-operator does not exist",
				fn: func() {
					ginkgo.By("Uninstall tidb-operator")
					oa.CleanOperatorOrDie(ocfg)
				},
			},
		}

		for _, test := range testCases {
			test := test
			ginkgo.It("tidb cluster should not be affected while "+test.name, func() {
				clusterName := "test"
				tc := fixture.GetTidbCluster(ns, clusterName, utilimage.TiDBLatest)
				utiltc.MustCreateTCWithComponentsReady(genericCli, oa, tc, 30*time.Minute, 15*time.Second)

				test.fn()

				ginkgo.By("Check tidb cluster is not affected")
				listOptions := metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(label.New().Instance(clusterName).Labels()).String(),
				}
				podList, err := c.CoreV1().Pods(ns).List(context.TODO(), listOptions)
				framework.ExpectNoError(err, "failed to list pods in ns %s", ns)
				err = wait.PollImmediate(time.Second*30, time.Minute*5, func() (bool, error) {
					var ok bool
					var err error
					log.Logf("check whether pods of cluster %q are changed", clusterName)
					ok, err = utilpod.PodsAreChanged(c, podList.Items)()
					if err != nil {
						log.Logf("ERROR: meet error during check pods of cluster %q are changed, err:%v", clusterName, err)
						return false, err
					}
					if ok {
						return true, nil
					}
					log.Logf("check whether pods of cluster %q are running", clusterName)
					newPodList, err := c.CoreV1().Pods(ns).List(context.TODO(), listOptions)
					if err != nil {
						return false, err
					}
					for _, pod := range newPodList.Items {
						if pod.Status.Phase != v1.PodRunning {
							return false, fmt.Errorf("pod %s/%s is not running", pod.Namespace, pod.Name)
						}
					}
					log.Logf("check whehter tidb cluster %q is connectable", clusterName)
					ok, err = utiltidb.TiDBIsConnectable(fw, ns, clusterName, "root", "")()
					if !ok || err != nil {
						// not connectable or some error happened
						return true, err
					}
					return false, nil
				})
				framework.ExpectEqual(err, wait.ErrWaitTimeout, "TiDB cluster is not affeteced")
			})
		}

		// In this test, we demonstrate and verify the recover process when a
		// node (and local storage on it) is permanently gone.
		//
		// In cloud, a node can be deleted manually or reclaimed by a
		// controller (e.g. auto scaling group if ReplaceUnhealthy not
		// suspended). Local storage on it will be permanently unaccessible.
		// Manual intervention is required to recover from this situation.
		// Basic steps will be:
		//
		// - for TiKV, delete associated store ID in PD
		//   - because we use network identity as store address, if we want to
		//   recover in place, we should delete the previous store at the same
		//   address. This requires us to set it to tombstone directly because
		//   the data is permanent lost, there is no way to delete it gracefully.
		//   - optionally, Advnaced StatefulSet can be used to recover with
		//   different network identity
		// - for PD, like TiKV we must delete its member from the cluster
		// - (EKS only) delete pvcs of failed pods
		//   - in EKS, failed pods on deleted node will be recreated because
		//   the node object is gone too (old pods is recycled by pod gc). But
		//   the newly created pods will be stuck at Pending state because
		//   associated PVs are invalid now. Pods will be recreated by
		//   tidb-operator again when we delete associated PVCs. New PVCs will
		//   be created by statefulset controller and pods will be scheduled to
		//   feasible nodes.
		//   - it's highly recommended to enable `setPVOwnerRef` in
		//   local-volume-provisioner, then orphan PVs will be garbaged
		//   collected and will not cause problem even if the name of deleted
		//   node is used again in the future.
		// - (GKE only, fixed path) nothing need to do
		//   - Because the node name does not change, old PVs can be used. Note
		//   that `setPVOwnerRef` cannot be enabled because the node object
		//   could get deleted if it takes too long for the instance to
		//   recreate.
		//   - Optionally, you can deleted failed pods to make them to start
		//   soon. This is due to exponential crash loop back off.
		// - (GKE only, unique paths) delete failed pods and associated PVCs/PVs
		//   - This is because even if the node name does not change, old PVs
		//   are invalid because unique volume paths are used. We must delete
		//   them all and wait for Kubernetes to rcreate and run again.
		//   - PVs must be deleted because the PVs are invalid and should not
		//   exist anymore. We can configure `setPVOwnerRef` to clean unused
		//   PVs when the node object is deleted, but the node object will not
		//   get deleted if the instance is recreated soon.
		//
		// Note that:
		// - We assume local storage is used, otherwise PV can be re-attached
		// the new node without problem.
		// - PD and TiKV must have at least 3 replicas, otherwise one node
		// deletion will cause permanent data loss and the cluster will be unrecoverable.
		// - Of course, this process can be automated by implementing a
		// controller integrated with cloud providers. It's outside the scope
		// of tidb-operator now.
		// - The same process can apply in bare-metal environment too when a
		// machine or local storage is permanently gone.
		//
		// Differences between EKS and GKE:
		//
		// - In EKS, a new node object with different name will be created for
		// the new machine.
		// - In GKE (1.11+), the node object are no longer recreated on
		// upgrade/repair even though the underlying instance is recreated and
		// local disks are wiped. However, the node object could get deleted by
		// cloud-controller-manager if it takes too long for the instance to
		// recreate.
		//
		// Related issues:
		// - https://github.com/pingcap/tidb-operator/issues/1546
		// - https://github.com/pingcap/tidb-operator/issues/408
		ginkgo.It("recover tidb cluster from node deletion", func() {
			supportedProviders := sets.NewString("aws", "gke")
			if !supportedProviders.Has(framework.TestContext.Provider) {
				e2eskipper.Skipf("current provider is not supported list %v, skipping", supportedProviders.List())
			}

			ginkgo.By("Make sure we have at least 3 schedulable nodes")
			nodeList, err := e2enode.GetReadySchedulableNodes(c)
			framework.ExpectNoError(err)
			gomega.Expect(len(nodeList.Items)).To(gomega.BeNumerically(">=", 3))

			ginkgo.By("Deploy a test cluster with 3 pd and tikv replicas")
			clusterName := "test"
			tc := fixture.GetTidbCluster(ns, clusterName, utilimage.TiDBLatest)
			tc.Spec.PD.Replicas = 3
			tc.Spec.PD.MaxFailoverCount = pointer.Int32Ptr(0)
			tc.Spec.TiDB.Replicas = 1
			tc.Spec.TiDB.MaxFailoverCount = pointer.Int32Ptr(0)
			tc.Spec.TiKV.Replicas = 3
			tc.Spec.TiKV.MaxFailoverCount = pointer.Int32Ptr(0)
			utiltc.MustCreateTCWithComponentsReady(genericCli, oa, tc, 30*time.Minute, 15*time.Second)

			ginkgo.By("By using tidb-scheduler, 3 TiKV/PD replicas should be on different nodes")
			allNodes := make(map[string]v1.Node)
			for _, node := range nodeList.Items {
				allNodes[node.Name] = node
			}
			allTiKVNodes := make(map[string]v1.Node)
			allPDNodes := make(map[string]v1.Node)
			listOptions := metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(label.New().Instance(clusterName).Labels()).String(),
			}
			podList, err := c.CoreV1().Pods(ns).List(context.TODO(), listOptions)
			framework.ExpectNoError(err, "failed to list pods in ns %s", ns)
			for _, pod := range podList.Items {
				if v, ok := pod.Labels[label.ComponentLabelKey]; !ok {
					log.Failf("pod %s/%s does not have component label key %q", pod.Namespace, pod.Name, label.ComponentLabelKey)
				} else if v == label.PDLabelVal {
					allPDNodes[pod.Name] = allNodes[pod.Spec.NodeName]
				} else if v == label.TiKVLabelVal {
					allTiKVNodes[pod.Name] = allNodes[pod.Spec.NodeName]
				} else {
					continue
				}
			}
			gomega.Expect(len(allPDNodes)).To(gomega.BeNumerically("==", 3), "the number of pd nodes should be 3")
			gomega.Expect(len(allTiKVNodes)).To(gomega.BeNumerically("==", 3), "the number of tikv nodes should be 3")

			ginkgo.By("Deleting a node")
			var nodeToDelete *v1.Node
			for _, node := range allTiKVNodes {
				if nodeToDelete == nil {
					nodeToDelete = &node
					break
				}
			}
			gomega.Expect(nodeToDelete).NotTo(gomega.BeNil())
			var pdPodsOnDeletedNode []v1.Pod
			var tikvPodsOnDeletedNode []v1.Pod
			var pvcNamesOnDeletedNode []string
			for _, pod := range podList.Items {
				if pod.Spec.NodeName == nodeToDelete.Name {
					if v, ok := pod.Labels[label.ComponentLabelKey]; ok {
						if v == label.PDLabelVal {
							pdPodsOnDeletedNode = append(pdPodsOnDeletedNode, pod)
						} else if v == label.TiKVLabelVal {
							tikvPodsOnDeletedNode = append(tikvPodsOnDeletedNode, pod)
						}
					}
					for _, volume := range pod.Spec.Volumes {
						if volume.PersistentVolumeClaim != nil {
							pvcNamesOnDeletedNode = append(pvcNamesOnDeletedNode, volume.PersistentVolumeClaim.ClaimName)
						}
					}
				}
			}
			gomega.Expect(len(tikvPodsOnDeletedNode)).To(gomega.BeNumerically(">=", 1), "the number of affected tikvs must be equal or greater than 1")
			err = framework.TestContext.CloudConfig.Provider.DeleteNode(nodeToDelete)
			framework.ExpectNoError(err, fmt.Sprintf("failed to delete node %q", nodeToDelete.Name))
			log.Logf("Node %q deleted", nodeToDelete.Name)

			ginkgo.By("Mark stores of failed tikv pods as tombstone")
			pdClient, cancel, err := proxiedpdclient.NewProxiedPDClient(secretLister, fw, ns, clusterName, false)
			framework.ExpectNoError(err, "failed to create proxied PD client")
			defer func() {
				if cancel != nil {
					cancel()
				}
			}()
			for _, pod := range tikvPodsOnDeletedNode {
				log.Logf("Mark tikv store of pod %s/%s as Tombstone", ns, pod.Name)
				err = wait.PollImmediate(time.Second*3, time.Minute, func() (bool, error) {
					storeID, err := utiltikv.GetStoreIDByPodName(cli, ns, clusterName, pod.Name)
					if err != nil {
						return false, nil
					}
					err = pdClient.SetStoreState(storeID, v1alpha1.TiKVStateTombstone)
					if err != nil {
						return false, nil
					}
					return true, nil
				})
				framework.ExpectNoError(err, "mark tikv store as Tombstone timeout")
			}
			ginkgo.By("Delete pd members")
			for _, pod := range pdPodsOnDeletedNode {
				log.Logf("Delete pd member of pod %s/%s", ns, pod.Name)
				err = wait.PollImmediate(time.Second*3, time.Minute, func() (bool, error) {
					err = pdClient.DeleteMember(pod.Name)
					if err != nil {
						return false, nil
					}
					return true, nil
				})
				framework.ExpectNoError(err, "delete pd members timeout")
			}
			cancel()
			cancel = nil

			if framework.TestContext.Provider == "aws" {
				// Local storage is gone with the node and local PVs on deleted
				// node will be unusable.
				// If `setPVOwnerRef` is enabled in local-volume-provisioner,
				// local PVs will be deleted when the node object is deleted
				// and permanently gone in apiserver when associated PVCs are
				// delete here.
				ginkgo.By("[AWS/EKS] Delete associated PVCs if they are bound with local PVs")
				localPVs := make([]string, 0)
				for _, pvcName := range pvcNamesOnDeletedNode {
					pvc, err := c.CoreV1().PersistentVolumeClaims(ns).Get(context.TODO(), pvcName, metav1.GetOptions{})
					if err != nil && !apierrors.IsNotFound(err) {
						log.Failf("apiserver error: %v", err)
					}
					if apierrors.IsNotFound(err) {
						continue
					}
					if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName == "local-storage" {
						// TODO check the localPVs as expected in someway?
						// SA4010: this result of append is never used, except maybe in other appends
						localPVs = append(localPVs, pvc.Spec.VolumeName) // nolint: staticcheck
						err = c.CoreV1().PersistentVolumeClaims(ns).Delete(context.TODO(), pvc.Name, metav1.DeleteOptions{})
						framework.ExpectNoError(err, "failed to delete pvc %s", pvc.Name)
					}
				}
			} else if framework.TestContext.Provider == "gke" {
				log.Logf("We are using fixed paths in local PVs in our e2e. PVs of the deleted node are usable though the underlying storage is empty now")
				// Because of pod exponential crash loop back off, we can
				// delete the failed pods to make it start soon.
				// Note that this is optional.
				ginkgo.By("Deleting the failed pods")
				for _, pod := range append(tikvPodsOnDeletedNode, pdPodsOnDeletedNode...) {
					framework.ExpectNoError(c.CoreV1().Pods(ns).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{}))
				}
			}

			ginkgo.By("Waiting for tidb cluster to be fully ready")
			err = oa.WaitForTidbClusterReady(tc, 5*time.Minute, 15*time.Second)
			framework.ExpectNoError(err, "wait for TidbCluster ready timeout: %v", tc)
		})

		// There is no guarantee but tidb pods should be assigned back to
		// previous nodes if no other pods to occupy the positions.
		// See docs/design-proposals/tidb-stable-scheduling.md
		ginkgo.It("[Feature: StableScheduling] TiDB pods should be scheduled to preivous nodes", func() {
			clusterName := "tidb-scheduling"
			tc := fixture.GetTidbCluster(ns, clusterName, utilimage.TiDBLatest)
			tc.Spec.PD.Replicas = 1
			tc.Spec.TiKV.Replicas = 1
			tc.Spec.TiDB.Replicas = 3
			utiltc.MustCreateTCWithComponentsReady(genericCli, oa, tc, 30*time.Minute, 15*time.Second)

			listOptions := metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(
					label.New().Instance(clusterName).Component(label.TiDBLabelVal).Labels()).String(),
			}
			oldPodList, err := c.CoreV1().Pods(ns).List(context.TODO(), listOptions)
			framework.ExpectNoError(err, "failed to list pods in ns %s", ns)

			ginkgo.By("Update tidb configuration")
			updateStrategy := v1alpha1.ConfigUpdateStrategyRollingUpdate
			err = controller.GuaranteedUpdate(genericCli, tc, func() error {
				tc.Spec.TiDB.Config.Set("token-limit", 2000)
				tc.Spec.TiDB.ConfigUpdateStrategy = &updateStrategy
				return nil
			})
			framework.ExpectNoError(err, "failed to update TidbCluster: %v", tc)

			ginkgo.By("Waiting for all tidb pods are recreated and assigned to the same node")
			getOldPodByName := func(pod *v1.Pod) *v1.Pod {
				for _, oldPod := range oldPodList.Items {
					if oldPod.Name == pod.Name {
						return &oldPod
					}
				}
				return nil
			}
			err = wait.PollImmediate(time.Second*5, time.Minute*15, func() (bool, error) {
				newPodList, err := c.CoreV1().Pods(ns).List(context.TODO(), listOptions)
				if err != nil && !apierrors.IsNotFound(err) {
					return false, err
				}
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				if len(newPodList.Items) != len(oldPodList.Items) {
					return false, nil
				}
				for _, newPod := range newPodList.Items {
					oldPod := getOldPodByName(&newPod)
					if oldPod == nil {
						return false, fmt.Errorf("found an unexpected pod: %q", newPod.Name)
					}
					if oldPod.UID == newPod.UID {
						// not recreated yet
						return false, nil
					}
					if oldPod.Spec.NodeName != newPod.Spec.NodeName {
						// recreated but assigned to another node
						return false, fmt.Errorf("pod %q recreated but not assigned to previous node %q, got %q", oldPod.Name, oldPod.Spec.NodeName, newPod.Spec.NodeName)
					}
				}
				return true, nil
			})
			framework.ExpectNoError(err, "wait for pod recreate timeout")
		})
	})

	ginkgo.Context("operator with short auto-failover periods", func() {
		var ocfg *tests.OperatorConfig
		var oa *tests.OperatorActions
		var genericCli client.Client
		failoverPeriod := time.Minute

		ginkgo.BeforeEach(func() {
			ocfg = &tests.OperatorConfig{
				Namespace:   ns,
				ReleaseName: "operator",
				Image:       cfg.OperatorImage,
				Tag:         cfg.OperatorTag,
				LogLevel:    "4",
				TestMode:    true,
				StringValues: map[string]string{
					"controllerManager.pdFailoverPeriod":      failoverPeriod.String(),
					"controllerManager.tidbFailoverPeriod":    failoverPeriod.String(),
					"controllerManager.tikvFailoverPeriod":    failoverPeriod.String(),
					"controllerManager.tiflashFailoverPeriod": failoverPeriod.String(),
				},
			}
			oa = tests.NewOperatorActions(cli, c, asCli, aggrCli, apiExtCli, tests.DefaultPollInterval, ocfg, e2econfig.TestConfig, fw, f)
			ginkgo.By("Installing CRDs")
			oa.CleanCRDOrDie()
			oa.CreateCRDOrDie(ocfg)
			ginkgo.By("Installing tidb-operator")
			oa.CleanOperatorOrDie(ocfg)
			oa.DeployOperatorOrDie(ocfg)
			var err error
			genericCli, err = client.New(config, client.Options{Scheme: scheme.Scheme})
			framework.ExpectNoError(err, "failed to create clientset")
		})

		ginkgo.It("[Feature: AutoFailover] PD: one replacement for one failed member and replacements should be deleted when failed members are recovered", func() {
			// TODO support aws (eks), kind
			supportedProviders := sets.NewString("gke")
			if !supportedProviders.Has(framework.TestContext.Provider) {
				e2eskipper.Skipf("current provider is not supported list %v, skipping", supportedProviders.List())
			}
			// Disable node auto repair, otherwise the node on which the
			// kubelet is not running will be recreated.
			defer utilcloud.EnableNodeAutoRepair()
			utilcloud.DisableNodeAutoRepair()
			clusterName := "failover"
			tc := fixture.GetTidbCluster(ns, clusterName, utilimage.TiDBLatest)
			tc.Spec.PD.Replicas = 3
			tc.Spec.TiKV.Replicas = 1
			tc.Spec.TiDB.Replicas = 1
			utiltc.MustCreateTCWithComponentsReady(genericCli, oa, tc, 30*time.Minute, 15*time.Second)

			ginkgo.By("Pre-create an invalid PVC to fail the auto-created failover member")
			invalidPVC := v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      fmt.Sprintf("pd-%s-pd-%d", clusterName, 3),
				},
				Spec: v1.PersistentVolumeClaimSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
					},
					StorageClassName: pointer.StringPtr("does-not-exist"),
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: *resource.NewQuantity(1, resource.BinarySI),
						},
					},
				},
			}
			_, err := c.CoreV1().PersistentVolumeClaims(ns).Create(context.TODO(), &invalidPVC, metav1.CreateOptions{})
			framework.ExpectNoError(err, "failed to create persistent volume claims: %v", invalidPVC)

			// We should stop the kubelet after failing the PD. Because
			// tidb-operator will try to recreate POD & PVC soon after a new
			// replacement is created.
			ginkgo.By("Fail a PD")
			listOptions := metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(
					label.New().Instance(clusterName).Component(label.PDLabelVal).Labels()).String(),
			}
			pdPodList, err := c.CoreV1().Pods(ns).List(context.TODO(), listOptions)
			framework.ExpectNoError(err, "failed to list pods in ns %s with selector %v", ns, listOptions)
			gomega.Expect(len(pdPodList.Items)).To(gomega.BeNumerically("==", 3), "the number of pd nodes should be 3")
			pod0 := pdPodList.Items[0]
			f.ExecCommandInContainer(pod0.Name, "pd", "sh", "-c", "rm -rf /var/lib/pd/member")
			// This command is to make sure kubelet is started after test finishes no matter it fails or not.
			defer func() {
				storageutils.KubeletCommand(storageutils.KStart, c, &pod0)
			}()
			storageutils.KubeletCommand(storageutils.KStop, c, &pod0)

			ginkgo.By("Wait for a replacement to be created")
			podName := controller.PDMemberName(clusterName) + "-3"
			err = wait.PollImmediate(time.Second*10, 2*failoverPeriod, func() (bool, error) {
				_, err := c.CoreV1().Pods(ns).Get(context.TODO(), podName, metav1.GetOptions{})
				if err != nil && !apierrors.IsNotFound(err) {
					return false, nil
				}
				return !apierrors.IsNotFound(err), nil
			})
			framework.ExpectNoError(err, "wait for pod to be replaced timeout")

			ginkgo.By("Wait for only one replacement to be created")
			err = wait.PollImmediate(time.Second*10, 1*time.Minute, func() (bool, error) {
				pdPodList, err := c.CoreV1().Pods(ns).List(context.TODO(), listOptions)
				if err != nil && !apierrors.IsNotFound(err) {
					return false, nil
				}
				if len(pdPodList.Items) != 4 {
					return true, nil
				}
				return false, nil
			})
			framework.ExpectEqual(err, wait.ErrWaitTimeout, "only one replacement should be created")

			ginkgo.By("Recover failed PD")
			storageutils.KubeletCommand(storageutils.KStart, c, &pod0)

			ginkgo.By("Wait for the failed PD to recover")
			err = pod.WaitTimeoutForPodRunningInNamespace(c, pod0.Name, ns, time.Minute*5)
			framework.ExpectNoError(err, "wait for failed pd to recover timeout")

			ginkgo.By("Wait for the replacement to be gone")
			err = pod.WaitForPodNotFoundInNamespace(c, podName, ns, time.Minute*5)
			framework.ExpectNoError(err, "failed to wait for replacement pod deleted")
		})

		ginkgo.It("[Feature: AutoFailover] TiDB: one replacement for one failed member and replacements should be deleted when failed members are recovered", func() {
			ginkgo.By("Make sure we have at least 3 schedulable nodes")
			nodeList, err := e2enode.GetReadySchedulableNodes(c)
			framework.ExpectNoError(err)
			gomega.Expect(len(nodeList.Items)).To(gomega.BeNumerically(">=", 3))

			clusterName := "failover"
			tc := fixture.GetTidbCluster(ns, clusterName, utilimage.TiDBLatest)
			tc.Spec.PD.Replicas = 1
			tc.Spec.TiKV.Replicas = 1
			tc.Spec.TiDB.Replicas = 2
			// We use special affinity requiremnets to make sure only 2 tidb pods can be scheduled.
			tc.Spec.TiDB.Affinity = &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
						NodeSelectorTerms: []v1.NodeSelectorTerm{
							{
								MatchExpressions: []v1.NodeSelectorRequirement{
									{
										Key:      v1.LabelHostname,
										Operator: v1.NodeSelectorOpIn,
										Values: []string{
											nodeList.Items[0].Name,
											nodeList.Items[1].Name,
										},
									},
								},
							},
						},
					},
				},
				PodAntiAffinity: &v1.PodAntiAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app.kubernetes.io/instance":  clusterName,
									"app.kubernetes.io/component": label.TiDBLabelVal,
								},
							},
							TopologyKey: v1.LabelHostname,
						},
					},
				},
			}
			utiltc.MustCreateTCWithComponentsReady(genericCli, oa, tc, 30*time.Minute, 15*time.Second)

			ginkgo.By("Increase replicas of TiDB from 2 to 3")
			err = controller.GuaranteedUpdate(genericCli, tc, func() error {
				tc.Spec.TiDB.Replicas = 3
				return nil
			})
			framework.ExpectNoError(err, "failed to update TidbCluster to scale out tidb")

			ginkgo.By("Wait for the new pod to be created")
			podName := controller.TiDBMemberName(clusterName) + "-2"
			err = wait.PollImmediate(time.Second*10, 1*time.Minute, func() (bool, error) {
				_, err := c.CoreV1().Pods(ns).Get(context.TODO(), podName, metav1.GetOptions{})
				if err != nil && !apierrors.IsNotFound(err) {
					return false, nil
				}
				return !apierrors.IsNotFound(err), nil
			})
			framework.ExpectNoError(err, "failed to wait for the new pod to be created")

			ginkgo.By("Make sure the new pod will not be scheduled")
			err = wait.PollImmediate(time.Second*10, 1*time.Minute, func() (bool, error) {
				pod, err := c.CoreV1().Pods(ns).Get(context.TODO(), podName, metav1.GetOptions{})
				if err != nil {
					if testutils.IsRetryableAPIError(err) {
						return false, nil
					}
					return false, err
				}
				_, condition := k8s.GetPodCondition(&pod.Status, v1.PodScheduled)
				if condition == nil || condition.Status != v1.ConditionTrue {
					return false, nil
				}
				return true, nil
			})
			framework.ExpectEqual(err, wait.ErrWaitTimeout, "the new pod should not be scheduled")

			listOptions := metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(
					label.New().Instance(clusterName).Component(label.TiDBLabelVal).Labels()).String(),
			}
			ginkgo.By("Wait for no new replacement will be created for non-scheduled TiDB pod")
			err = wait.PollImmediate(time.Second*10, 2*time.Minute, func() (bool, error) {
				pdPodList, err := c.CoreV1().Pods(ns).List(context.TODO(), listOptions)
				if err != nil && !apierrors.IsNotFound(err) {
					return false, nil
				}
				if len(pdPodList.Items) != 3 {
					return true, nil
				}
				return false, nil
			})
			framework.ExpectEqual(err, wait.ErrWaitTimeout, "no new replacement should be created for non-scheduled TiDB pod")

			ginkgo.By("Fix the TiDB scheduling requirements")
			err = controller.GuaranteedUpdate(genericCli, tc, func() error {
				tc.Spec.TiDB.Affinity = nil
				return nil
			})
			framework.ExpectNoError(err, "failed to update TidbCluster for tidb affinity")

			err = oa.WaitForTidbClusterReady(tc, 30*time.Minute, 15*time.Second)
			framework.ExpectNoError(err, "wait for TidbCluster ready timeout: %v", tc)

			ginkgo.By(fmt.Sprintf("Fail the TiDB pod %q", podName))
			patch := []byte(`
{
	"spec": {
		"containers": [
			{
				"name": "tidb",
				"image": "pingcap/does-not-exist:latest"
			}
		]
	}
}`)
			_, err = c.CoreV1().Pods(ns).Patch(context.TODO(), podName, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
			framework.ExpectNoError(err, "failed to patch pod with patch: %v", patch)

			err = wait.PollImmediate(time.Second*10, 1*time.Minute, func() (bool, error) {
				pod, err := c.CoreV1().Pods(ns).Get(context.TODO(), podName, metav1.GetOptions{})
				if err != nil {
					// TODO: should do this in wait.Poll and wait.PollImmediate
					if testutils.IsRetryableAPIError(err) {
						return false, nil
					}
					return false, err
				}
				return !k8s.IsPodReady(pod), nil
			})
			framework.ExpectNoError(err, "wait for patched pod ready timeout")

			ginkgo.By("Wait for a replacement to be created")
			newPodName := controller.TiDBMemberName(clusterName) + "-3"
			err = wait.PollImmediate(time.Second*10, 2*failoverPeriod, func() (bool, error) {
				_, err := c.CoreV1().Pods(ns).Get(context.TODO(), newPodName, metav1.GetOptions{})
				if err != nil && !apierrors.IsNotFound(err) {
					return false, nil
				}
				return !apierrors.IsNotFound(err), nil
			})
			framework.ExpectNoError(err, "failed to wait for failover tidb pod")

			ginkgo.By("Wait for only one replacement to be created")
			err = wait.PollImmediate(time.Second*10, 1*time.Minute, func() (bool, error) {
				podList, err := c.CoreV1().Pods(ns).List(context.TODO(), listOptions)
				if err != nil && !apierrors.IsNotFound(err) {
					return false, nil
				}
				if len(podList.Items) != 4 {
					return true, nil
				}
				return false, nil
			})
			framework.ExpectEqual(err, wait.ErrWaitTimeout, "only one replacement should be created")

			ginkgo.By(fmt.Sprintf("Fix the TiDB pod %q", podName))
			err = c.CoreV1().Pods(ns).Delete(context.TODO(), podName, metav1.DeleteOptions{})
			framework.ExpectNoError(err, "failed to delete tidb pod %s/%s", ns, podName)

			ginkgo.By("Wait for the replacement to be gone")
			err = pod.WaitForPodNotFoundInNamespace(c, newPodName, ns, time.Minute*5)
			framework.ExpectNoError(err, "failed to wait for replacement pod to be deleted")
		})

		// https://github.com/pingcap/tidb-operator/issues/2739
		// TODO: this should be a regression type
		ginkgo.It("[Feature: AutoFailover] Failover can work if a store fails to update", func() {
			clusterName := "scale"
			tc := fixture.GetTidbCluster(ns, clusterName, utilimage.TiDBLatest)
			tc.Spec.PD.Replicas = 1
			// By default, PD set the state of disconnected store to Down
			// after 30 minutes. Use a short time in testing.
			tc.Spec.PD.Config.Set("schedule.max-store-down-time", "1m")
			tc.Spec.TiKV.Replicas = 3
			tc.Spec.TiDB.Replicas = 1
			utiltc.MustCreateTCWithComponentsReady(genericCli, oa, tc, 30*time.Minute, 15*time.Second)

			ginkgo.By("Fail a TiKV store")
			podName := controller.TiKVMemberName(clusterName) + "-1"
			f.ExecCommandInContainer(podName, "tikv", "sh", "-c", "rm -rf /var/lib/tikv/*")

			ginkgo.By("Waiting for the store to be in Down state")
			err := utiltidbcluster.WaitForTCCondition(cli, tc.Namespace, tc.Name, time.Minute*5, time.Second*10,
				func(tc *v1alpha1.TidbCluster) (bool, error) {
					for _, store := range tc.Status.TiKV.Stores {
						if store.PodName == podName && store.State == v1alpha1.TiKVStateDown {
							return true, nil
						}
					}
					return false, nil
				})
			framework.ExpectNoError(err, "failed to wait for ")

			ginkgo.By("Update TiKV configuration")
			updateStrategy := v1alpha1.ConfigUpdateStrategyRollingUpdate
			err = controller.GuaranteedUpdate(genericCli, tc, func() error {
				tc.Spec.TiKV.Config.Set("log-level", "info")
				tc.Spec.TiKV.ConfigUpdateStrategy = &updateStrategy
				return nil
			})
			framework.ExpectNoError(err, "failed to update tikv configuration")

			ginkgo.By("Waiting for the store to be put into failure stores")
			err = utiltidbcluster.WaitForTCCondition(cli, tc.Namespace, tc.Name, time.Minute*5, time.Second*10,
				func(tc *v1alpha1.TidbCluster) (bool, error) {
					for _, failureStore := range tc.Status.TiKV.FailureStores {
						if failureStore.PodName == podName {
							return true, nil
						}
					}
					return false, nil
				})
			framework.ExpectNoError(err, "failed to wait for the store to be put into failure stores")

			ginkgo.By("Waiting for the new pod to be created")
			newPodName := controller.TiKVMemberName(clusterName) + "-3"
			err = wait.PollImmediate(time.Second*10, 1*time.Minute, func() (bool, error) {
				_, err := c.CoreV1().Pods(ns).Get(context.TODO(), newPodName, metav1.GetOptions{})
				if err != nil && !apierrors.IsNotFound(err) {
					return false, nil
				}
				return !apierrors.IsNotFound(err), nil
			})
			framework.ExpectNoError(err, "failed to wait for the new pod to be created")
		})

		// https://github.com/pingcap/tidb-operator/issues/2739
		// TODO: this should be a regression type
		ginkgo.It("[Feature: AutoFailover] Failover can work if a pd fails to update", func() {
			clusterName := "scale"
			tc := fixture.GetTidbCluster(ns, clusterName, utilimage.TiDBLatest)
			tc.Spec.PD.Replicas = 3
			tc.Spec.TiKV.Replicas = 1
			tc.Spec.TiDB.Replicas = 1
			utiltc.MustCreateTCWithComponentsReady(genericCli, oa, tc, 30*time.Minute, 15*time.Second)

			ginkgo.By("Fail a PD")
			podName := controller.PDMemberName(clusterName) + "-1"
			f.ExecCommandInContainer(podName, "pd", "sh", "-c", "rm -rf /var/lib/pd/*")

			ginkgo.By("Waiting for the pd to be in unhealthy state")
			err := utiltidbcluster.WaitForTCCondition(cli, tc.Namespace, tc.Name, time.Minute*5, time.Second*10,
				func(tc *v1alpha1.TidbCluster) (bool, error) {
					for _, member := range tc.Status.PD.Members {
						if member.Name == podName && !member.Health {
							return true, nil
						}
					}
					return false, nil
				})
			framework.ExpectNoError(err, "failed to wait for the pd to be in unhealthy state")

			ginkgo.By("Update PD configuration")
			updateStrategy := v1alpha1.ConfigUpdateStrategyRollingUpdate
			err = controller.GuaranteedUpdate(genericCli, tc, func() error {
				tc.Spec.PD.Config.Set("log.level", "info")
				tc.Spec.PD.ConfigUpdateStrategy = &updateStrategy
				return nil
			})
			framework.ExpectNoError(err, "failed to update pd configuration")

			ginkgo.By("Waiting for the pd to be put into failure members")
			err = utiltidbcluster.WaitForTCCondition(cli, tc.Namespace, tc.Name, time.Minute*5, time.Second*10,
				func(tc *v1alpha1.TidbCluster) (bool, error) {
					for _, failureMember := range tc.Status.PD.FailureMembers {
						if failureMember.PodName == podName {
							return true, nil
						}
					}
					return false, nil
				})
			framework.ExpectNoError(err, "failed to wait for the pd to be put into failure members")

			ginkgo.By("Waiting for the new pod to be created")
			newPodName := controller.PDMemberName(clusterName) + "-3"
			err = wait.PollImmediate(time.Second*10, 1*time.Minute, func() (bool, error) {
				_, err := c.CoreV1().Pods(ns).Get(context.TODO(), newPodName, metav1.GetOptions{})
				if err != nil && !apierrors.IsNotFound(err) {
					return false, nil
				}
				return !apierrors.IsNotFound(err), nil
			})
			framework.ExpectNoError(err, "failed to wait for the new pod to be created")
		})
	})

	ginkgo.Context("[Feature: AdvancedStatefulSet][Feature: AutoFailover] operator with advanced statefulset and short auto-failover periods", func() {
		var ocfg *tests.OperatorConfig
		var oa *tests.OperatorActions
		var genericCli client.Client
		failoverPeriod := time.Minute

		ginkgo.BeforeEach(func() {
			ocfg = &tests.OperatorConfig{
				Namespace:   ns,
				ReleaseName: "operator",
				Image:       cfg.OperatorImage,
				Tag:         cfg.OperatorTag,
				LogLevel:    "4",
				TestMode:    true,
				StringValues: map[string]string{
					"controllerManager.pdFailoverPeriod":      failoverPeriod.String(),
					"controllerManager.tidbFailoverPeriod":    failoverPeriod.String(),
					"controllerManager.tikvFailoverPeriod":    failoverPeriod.String(),
					"controllerManager.tiflashFailoverPeriod": failoverPeriod.String(),
				},
				Features: []string{
					"AdvancedStatefulSet=true",
				},
			}
			oa = tests.NewOperatorActions(cli, c, asCli, aggrCli, apiExtCli, tests.DefaultPollInterval, ocfg, e2econfig.TestConfig, fw, f)
			ginkgo.By("Installing CRDs")
			oa.CleanCRDOrDie()
			oa.CreateCRDOrDie(ocfg)
			ginkgo.By("Installing tidb-operator")
			oa.CleanOperatorOrDie(ocfg)
			oa.DeployOperatorOrDie(ocfg)
			var err error
			genericCli, err = client.New(config, client.Options{Scheme: scheme.Scheme})
			framework.ExpectNoError(err, "failed to create clientset")
		})

		// https://github.com/pingcap/tidb-operator/issues/1464
		ginkgo.It("delete the failed pod via delete-slots feature of Advanced Statefulset after failover", func() {
			ginkgo.By("Make sure we have at least 3 schedulable nodes")
			nodeList, err := e2enode.GetReadySchedulableNodes(c)
			framework.ExpectNoError(err)
			gomega.Expect(len(nodeList.Items)).To(gomega.BeNumerically(">=", 3))

			clusterName := "failover"
			tc := fixture.GetTidbCluster(ns, clusterName, utilimage.TiDBLatest)
			tc.Spec.SchedulerName = ""
			tc.Spec.PD.Replicas = 1
			tc.Spec.PD.Config.Set("schedule.max-store-down-time", "1m")
			tc.Spec.TiDB.Replicas = 1
			tc.Spec.TiKV.Replicas = 3

			ginkgo.By("Waiting for the tidb cluster to become ready")
			utiltc.MustCreateTCWithComponentsReady(genericCli, oa, tc, 30*time.Minute, 15*time.Second)

			ginkgo.By("Fail a TiKV store")
			podName := controller.TiKVMemberName(clusterName) + "-1"
			f.ExecCommandInContainer(podName, "tikv", "sh", "-c", "rm -rf /var/lib/tikv/*")

			ginkgo.By("Waiting for the store to be put into failure stores")
			err = utiltidbcluster.WaitForTCCondition(cli, tc.Namespace, tc.Name, time.Minute*5, time.Second*10,
				func(tc *v1alpha1.TidbCluster) (bool, error) {
					for _, failureStore := range tc.Status.TiKV.FailureStores {
						if failureStore.PodName == podName {
							return true, nil
						}
					}
					return false, nil
				})
			framework.ExpectNoError(err, "failed to wait for the store to be put into failure stores")

			ginkgo.By("Waiting for the new pod to be created")
			newPodName := controller.TiKVMemberName(clusterName) + "-3"
			err = wait.PollImmediate(time.Second*10, 1*time.Minute, func() (bool, error) {
				_, err := c.CoreV1().Pods(ns).Get(context.TODO(), newPodName, metav1.GetOptions{})
				if err != nil && !apierrors.IsNotFound(err) {
					return false, nil
				}
				return !apierrors.IsNotFound(err), nil
			})
			framework.ExpectNoError(err, "failed to wait for the new pod to be created")

			ginkgo.By(fmt.Sprintf("Deleting the failed pod %q via delete-slots", podName))
			err = controller.GuaranteedUpdate(genericCli, tc, func() error {
				if tc.Annotations == nil {
					tc.Annotations = map[string]string{}
				}
				tc.Annotations[label.AnnTiKVDeleteSlots] = mustToString(sets.NewInt32(1))
				return nil
			})
			framework.ExpectNoError(err, "failed to delete failed pod %q via delete-slots", podName)

			ginkgo.By(fmt.Sprintf("Waiting for the failed pod %q to be gone", podName))
			err = pod.WaitForPodNotFoundInNamespace(c, podName, ns, time.Minute*5)
			framework.ExpectNoError(err, "failed to wait for the failed pod %q to be gone", podName)

			ginkgo.By("Waiting for the record of failed pod to be removed from failure stores")
			err = utiltidbcluster.WaitForTCCondition(cli, tc.Namespace, tc.Name, time.Minute*5, time.Second*10,
				func(tc *v1alpha1.TidbCluster) (bool, error) {
					exist := false
					for _, failureStore := range tc.Status.TiKV.FailureStores {
						if failureStore.PodName == podName {
							exist = true
						}
					}
					return !exist, nil
				})
			framework.ExpectNoError(err, "failed to wait for the record of failed pod to be removed from failure stores")

			ginkgo.By("Waiting for the tidb cluster to become ready")
			err = utiltidbcluster.WaitForTCConditionReady(cli, tc.Namespace, tc.Name, time.Minute*30, 0)
			framework.ExpectNoError(err, "failed to wait for TidbCluster ready: %v", tc)
		})
	})
*/
