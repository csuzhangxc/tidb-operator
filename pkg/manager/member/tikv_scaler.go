// Copyright 2018 PingCAP, Inc.
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

package member

import (
	"fmt"
	"strconv"
	"time"

	"github.com/pingcap/tidb-operator/pkg/apis/label"
	"github.com/pingcap/tidb-operator/pkg/apis/pingcap/v1alpha1"
	"github.com/pingcap/tidb-operator/pkg/controller"
	"github.com/pingcap/tidb-operator/pkg/third_party/k8s"
	"github.com/pingcap/tidb-operator/pkg/util"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

const (
	defaultEvictLeaderTimeoutWhenScaleIn = time.Minute * 5
)

type tikvScaler struct {
	generalScaler
}

// NewTiKVScaler returns a tikv Scaler
func NewTiKVScaler(deps *controller.Dependencies) *tikvScaler {
	return &tikvScaler{generalScaler: generalScaler{deps: deps}}
}

func (s *tikvScaler) Scale(meta metav1.Object, oldSet *apps.StatefulSet, newSet *apps.StatefulSet) error {
	scaling, _, _, _ := scaleOne(oldSet, newSet)
	if scaling > 0 {
		return s.ScaleOut(meta, oldSet, newSet)
	} else if scaling < 0 {
		return s.ScaleIn(meta, oldSet, newSet)
	}
	// we only sync auto scaler annotations when we are finishing syncing scaling
	return nil
}

func (s *tikvScaler) ScaleOut(meta metav1.Object, oldSet *apps.StatefulSet, newSet *apps.StatefulSet) error {
	tc, ok := meta.(*v1alpha1.TidbCluster)
	if !ok {
		klog.Errorf("tikvScaler.ScaleOut: failed to convert cluster %s/%s, scale out will do nothing", meta.GetNamespace(), meta.GetName())
		return nil
	}

	scaleOutParallelism := tc.Spec.TiKV.GetScaleOutParallelism()
	_, ordinals, replicas, deleteSlots := scaleMulti(oldSet, newSet, scaleOutParallelism)
	klog.Infof("scaling out tikv statefulset %s/%s, ordinal: %v (replicas: %d, scale out parallelism: %d, delete slots: %v)",
		oldSet.Namespace, oldSet.Name, ordinals, replicas, scaleOutParallelism, deleteSlots.List())

	var (
		errs                         []error
		finishedOrdinals             = sets.NewInt32()
		updateReplicasAndDeleteSlots bool
	)
	for _, ordinal := range ordinals {
		err := s.scaleOutOne(tc, ordinal)
		if err != nil {
			errs = append(errs, err)
		} else {
			finishedOrdinals.Insert(ordinal)
			updateReplicasAndDeleteSlots = true
		}
	}
	if updateReplicasAndDeleteSlots {
		setReplicasAndDeleteSlotsByFinished(scalingOutFlag, newSet, oldSet, ordinals, finishedOrdinals)
	} else {
		resetReplicas(newSet, oldSet)
	}
	return errorutils.NewAggregate(errs)
}

func (s *tikvScaler) scaleOutOne(tc *v1alpha1.TidbCluster, ordinal int32) error {
	pvcName := fmt.Sprintf("tikv-%s-tikv-%d", tc.GetName(), ordinal)
	_, err := s.deps.PVCLister.PersistentVolumeClaims(tc.GetNamespace()).Get(pvcName)
	if err == nil {
		_, err = s.deleteDeferDeletingPVC(tc, v1alpha1.TiKVMemberType, ordinal)
		if err != nil {
			return err
		}
		return controller.RequeueErrorf("tikv.ScaleOut, cluster %s/%s ready to scale out, wait for next round", tc.GetNamespace(), tc.GetName())
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("tikv.ScaleOut, cluster %s/%s failed to fetch pvc informaiton, err:%v", tc.GetNamespace(), tc.GetName(), err)
	}
	return nil
}

func (s *tikvScaler) ScaleIn(meta metav1.Object, oldSet *apps.StatefulSet, newSet *apps.StatefulSet) error {
	scaleInTime := time.Now()
	tc, ok := meta.(*v1alpha1.TidbCluster)
	if !ok {
		klog.Errorf("tikvScaler.ScaleIn: failed to convert cluster %s/%s, scale in will do nothing", meta.GetNamespace(), meta.GetName())
		return nil
	}

	scaleInParallelism := tc.Spec.TiKV.GetScaleInParallelism()

	_, ordinals, replicas, deleteSlots := scaleMulti(oldSet, newSet, scaleInParallelism)

	klog.Infof("scaling in tikv statefulset %s/%s, ordinals: %v (replicas: %d, delete slots: %v), scaleInParallelism: %v, scaleInTime: %v",
		oldSet.Namespace, oldSet.Name, ordinals, replicas, deleteSlots.List(), scaleInParallelism, scaleInTime.Format(time.RFC3339))

	var (
		upTikvStoreCount    int
		deletedUpStoreTotal int
		skipPreCheck        bool
		maxReplicas         int
	)
	if !tc.TiKVBootStrapped() {
		klog.Infof("TiKV of Cluster %s/%s is not bootstrapped yet, skip pre check when scale in TiKV", tc.Namespace, tc.Name)
		skipPreCheck = true
	} else {
		var err error
		pdClient := controller.GetPDClient(s.deps.PDControl, tc)
		storesInfo, err := pdClient.GetStores()
		if err != nil {
			return fmt.Errorf("failed to get stores info in TidbCluster %s/%s", tc.GetNamespace(), tc.GetName())
		}
		config, err := pdClient.GetConfig()
		if err != nil {
			return fmt.Errorf("failed to get config in TidbCluster %s/%s", tc.GetNamespace(), tc.GetName())
		}
		maxReplicas = int(*(config.Replication.MaxReplicas))
		// filter out TiFlash
		for _, store := range storesInfo.Stores {
			if store.Store != nil && store.Store.StateName == v1alpha1.TiKVStateUp && util.MatchLabelFromStoreLabels(store.Store.Labels, label.TiKVLabelVal) {
				upTikvStoreCount++
			}
		}
	}

	var (
		errs                         []error
		finishedOrdinals             = sets.NewInt32()
		updateReplicasAndDeleteSlots bool
	)
	// since first call of scale-in would give a requeue error,
	// try to do scale for all the stores here, so that we can batch requeue error,
	// record finished status for replicas and delete slots update.
	for _, ordinal := range ordinals {
		deletedUpStore, err := s.scaleInOne(tc, skipPreCheck, upTikvStoreCount, deletedUpStoreTotal, maxReplicas, ordinal, scaleInTime)
		// deletedUpStore stands for the count of store scaled during scaleInOne
		// should add it before check error
		deletedUpStoreTotal += deletedUpStore
		if err != nil {
			errs = append(errs, err)
		} else {
			finishedOrdinals.Insert(ordinal)
			updateReplicasAndDeleteSlots = true
		}
	}

	if updateReplicasAndDeleteSlots {
		setReplicasAndDeleteSlotsByFinished(scalingInFlag, newSet, oldSet, ordinals, finishedOrdinals)
	} else {
		resetReplicas(newSet, oldSet)
	}
	return errorutils.NewAggregate(errs)
}

func (s *tikvScaler) scaleInOne(tc *v1alpha1.TidbCluster, skipPreCheck bool, upTikvStoreCount, deletedUpStoreCount, maxReplicas int, ordinal int32, currentTime time.Time) (deletedUpStore int, err error) {
	tcName := tc.GetName()
	ns := tc.GetNamespace()
	podName := ordinalPodName(v1alpha1.TiKVMemberType, tcName, ordinal)
	pod, err := s.deps.PodLister.Pods(ns).Get(podName)
	if err != nil {
		return deletedUpStore, fmt.Errorf("tikvScaler.ScaleIn: failed to get pods %s for cluster %s/%s, error: %s", podName, ns, tcName, err)
	}

	if !skipPreCheck && !s.preCheckUpStores(tc, podName, upTikvStoreCount, deletedUpStoreCount, maxReplicas) {
		return deletedUpStore, fmt.Errorf("tikvScaler.ScaleIn: failed to pass up stores check , pod %s, cluster %s/%s", podName, ns, tcName)
	}

	// Below code depends on tikv StoreIDLabelKey & AnnTiKVNoActiveStoreSince to be correctly updated, so manually
	// update it once here (to avoid a dependency on metaManager to sync it first instead)
	pod, err = s.deps.PodControl.UpdateMetaInfo(tc, pod)
	if err != nil {
		klog.Errorf("tikvScaler.ScaleIn: failed to update pod MetaInfo for, pod %s, cluster %s/%s", podName, ns, tcName)
		return deletedUpStore, nil
	}

	// call PD API to delete the store of the TiKV Pod to be scaled in
	for _, store := range tc.Status.TiKV.Stores {
		if store.PodName == podName {
			state := store.State
			if pod.Labels[label.StoreIDLabelKey] == "" {
				return deletedUpStore, fmt.Errorf("StoreID not yet updated on pod label")
			}
			id, err := strconv.ParseUint(store.ID, 10, 64)
			if err != nil {
				return deletedUpStore, err
			}
			pdc := controller.GetPDClient(s.deps.PDControl, tc)

			var startTime *time.Time
			startStr, ok := pod.Annotations[label.AnnoScaleInTime]
			if ok {
				t, err := time.Parse(time.RFC3339, startStr)
				if err != nil {
					klog.Warningf("tikvScaler.ScaleIn: cannot parse annotation %s in pod %s, cluster %s/%s: %v", label.AnnoScaleInTime, podName, ns, tcName, err)
					// use current time as startInTime
					startTime = &currentTime
				} else {
					startTime = &t
				}
			} else {
				startTime = &currentTime
			}
			if err := ensureScaleInTimeAnnoInPod(tc, pod, s.deps.PodControl, startTime.Format(time.RFC3339)); err != nil {
				return deletedUpStore, fmt.Errorf("cannot add annotation to pod: %w", err)
			}

			var leaderEvictedOrTimeout bool
			if startTime.Add(defaultEvictLeaderTimeoutWhenScaleIn).Before(currentTime) {
				leaderEvictedOrTimeout = true
			}
			if store.LeaderCount == 0 {
				leaderEvictedOrTimeout = true
			}
			if !leaderEvictedOrTimeout {
				schedulerMap, err := pdc.GetEvictLeaderSchedulersForStores(id)
				if err != nil {
					return deletedUpStore, fmt.Errorf("cannot get scheduler of store %v: %w", id, err)
				}
				if _, ok := schedulerMap[id]; !ok {
					if err := pdc.BeginEvictLeader(id); err != nil {
						return deletedUpStore, fmt.Errorf("cannot evict leaders of store %v: %w", id, err)
					}
				}
			}

			if state != v1alpha1.TiKVStateOffline && leaderEvictedOrTimeout {
				if err := pdc.DeleteStore(id); err != nil {
					klog.Errorf("tikvScaler.ScaleIn: failed to delete store %d, %v", id, err)
					return deletedUpStore, err
				}
				klog.Infof("tikvScaler.ScaleIn: delete store %d for tikv %s/%s successfully", id, ns, podName)
				if state == v1alpha1.TiKVStateUp {
					deletedUpStore++
				}
			}
			return deletedUpStore, controller.RequeueErrorf("TiKV %s/%s store %d is still in cluster, state: %s", ns, podName, id, state)
		}
	}

	// If the store state turns to Tombstone, add defer deleting annotation to the PVCs of the Pod
	for storeID, store := range tc.Status.TiKV.TombstoneStores {
		if store.PodName != podName {
			continue
		}
		if pod.Labels[label.StoreIDLabelKey] != storeID {
			klog.Warningf("TiKV %s/%s store %s in status is not equal with store %s in label",
				ns, podName, storeID, pod.Labels[label.StoreIDLabelKey])
			continue
		}

		id, err := strconv.ParseUint(store.ID, 10, 64)
		if err != nil {
			return deletedUpStore, err
		}

		// TODO: double check if store is really not in Up/Offline/Down state
		klog.Infof("TiKV %s/%s store %d becomes tombstone", ns, podName, id)

		pvcs, err := util.ResolvePVCFromPod(pod, s.deps.PVCLister)
		if err != nil {
			return deletedUpStore, fmt.Errorf("tikvScaler.ScaleIn: failed to get pvcs for pod %s/%s in tc %s/%s, error: %s", ns, pod.Name, ns, tcName, err)
		}
		for _, pvc := range pvcs {
			if err := addDeferDeletingAnnoToPVC(tc, pvc, s.deps.PVCControl, currentTime.Format(time.RFC3339)); err != nil {
				return deletedUpStore, err
			}
		}

		// endEvictLeader for TombStone stores
		if err = endEvictLeaderbyStoreID(s.deps, tc, id); err != nil {
			return deletedUpStore, err
		}
		return deletedUpStore, nil
	}

	// When store not found in TidbCluster status, there are two possible situations:
	// 1. TiKV has already joined cluster but status not synced yet.
	//    In this situation return error to wait for another round for safety.
	// 2. TiKV pod is not ready, such as in pending state.
	//    In this situation we should delete this TiKV pod immediately to avoid blocking the subsequent operations.
	// 3. TiKV pod has a valid store in label, but no active stores from PD (via status).
	//    In this situation we assume that store has been Tombstone'd but pd has gc'ed it, so not available in
	//    TombstoneStores. We delete the pod in this case.
	if !k8s.IsPodReady(pod) {
		if tc.TiKVBootStrapped() {
			safeTimeDeadline := pod.CreationTimestamp.Add(5 * s.deps.CLIConfig.ResyncDuration)
			if time.Now().Before(safeTimeDeadline) {
				// Wait for 5 resync periods to ensure that the following situation does not occur:
				//
				// The tikv pod starts for a while, but has not synced its status, and then the pod becomes not ready.
				// Here we wait for 5 resync periods to ensure that the status of this tikv pod has been synced.
				// After this period of time, if there is still no information about this tikv in TidbCluster status,
				// then we can be sure that this tikv has never been added to the tidb cluster.
				// So we can scale in this tikv pod safely.
				return deletedUpStore, fmt.Errorf("TiKV %s/%s is not ready, wait for 5 resync periods to sync its status", ns, podName)
			}
			klog.Warningf("TiKV %s/%s is not ready, scale in it after waiting for 5 resync periods", ns, podName)
		}

		pvcs, err := util.ResolvePVCFromPod(pod, s.deps.PVCLister)
		if err != nil {
			return deletedUpStore, fmt.Errorf("tikvScaler.ScaleIn: failed to get pvcs for pod %s/%s in tc %s/%s, error: %s", ns, pod.Name, ns, tcName, err)
		}
		for _, pvc := range pvcs {
			if err := addDeferDeletingAnnoToPVC(tc, pvc, s.deps.PVCControl); err != nil {
				return deletedUpStore, err
			}
		}
		return deletedUpStore, nil
	}

	noActiveStoreSinceAnnValue, exists := pod.Annotations[label.AnnTiKVNoActiveStoreSince]
	if exists {
		noActiveStoreSinceTime, err := time.Parse(time.RFC3339, noActiveStoreSinceAnnValue)
		if err == nil {
			// Wait for 5 resync periods to ensure that the store is really not showing up in status.
			if metav1.Now().Time.After(noActiveStoreSinceTime.Add(5 * s.deps.CLIConfig.ResyncDuration)) {
				pvcs, err := util.ResolvePVCFromPod(pod, s.deps.PVCLister)
				if err != nil {
					return deletedUpStore, fmt.Errorf("tikvScaler.ScaleIn: failed to get pvcs for pod %s/%s in tc %s/%s, error: %s", ns, pod.Name, ns, tcName, err)
				}
				for _, pvc := range pvcs {
					if err := addDeferDeletingAnnoToPVC(tc, pvc, s.deps.PVCControl); err != nil {
						return deletedUpStore, err
					}
				}
				return deletedUpStore, nil
			}
		}
	}

	return deletedUpStore, fmt.Errorf("TiKV %s/%s not found in cluster", ns, podName)
}

func (s *tikvScaler) preCheckUpStores(tc *v1alpha1.TidbCluster, podName string, upTikvStoreCount, deletedUpStoreCount, maxReplicas int) bool {
	if !tc.TiKVBootStrapped() {
		klog.Infof("TiKV of Cluster %s/%s is not bootstrapped yet, skip pre check when scale in TiKV", tc.Namespace, tc.Name)
		return true
	}

	// decrease deleted store in this round.
	upNumber := upTikvStoreCount - deletedUpStoreCount

	// get the state of the store which is about to be scaled in
	storeState := ""
	for _, store := range tc.Status.TiKV.Stores {
		if store.PodName == podName {
			storeState = store.State
		}
	}

	if upNumber < maxReplicas {
		errMsg := fmt.Sprintf("the number of stores in Up state of TidbCluster [%s/%s] is %d, less than MaxReplicas in PD configuration(%d), can't scale in TiKV, podname %s ", tc.GetNamespace(), tc.GetName(), upNumber, maxReplicas, podName)
		klog.Error(errMsg)
		s.deps.Recorder.Event(tc, v1.EventTypeWarning, "FailedScaleIn", errMsg)
		return false
	} else if upNumber == maxReplicas {
		if storeState == v1alpha1.TiKVStateUp {
			errMsg := fmt.Sprintf("can't scale in TiKV of TidbCluster [%s/%s], cause the number of up stores is equal to MaxReplicas in PD configuration(%d), and the store in Pod %s which is going to be deleted is up too. MaxReplicas can be update online using pd-ctl or SQL statements, refer to https://docs.pingcap.com/tidb/stable/dynamic-config", tc.GetNamespace(), tc.GetName(), maxReplicas, podName)
			klog.Error(errMsg)
			s.deps.Recorder.Event(tc, v1.EventTypeWarning, "FailedScaleIn", errMsg)
			return false
		}
	}

	return true
}

type fakeTiKVScaler struct{}

// NewFakeTiKVScaler returns a fake tikv Scaler
func NewFakeTiKVScaler() Scaler {
	return &fakeTiKVScaler{}
}

func (s *fakeTiKVScaler) Scale(meta metav1.Object, oldSet *apps.StatefulSet, newSet *apps.StatefulSet) error {
	if *newSet.Spec.Replicas > *oldSet.Spec.Replicas {
		return s.ScaleOut(meta, oldSet, newSet)
	} else if *newSet.Spec.Replicas < *oldSet.Spec.Replicas {
		return s.ScaleIn(meta, oldSet, newSet)
	}
	return nil
}

func (s *fakeTiKVScaler) ScaleOut(_ metav1.Object, oldSet *apps.StatefulSet, newSet *apps.StatefulSet) error {
	setReplicasAndDeleteSlots(newSet, *oldSet.Spec.Replicas+1, nil)
	return nil
}

func (s *fakeTiKVScaler) ScaleIn(_ metav1.Object, oldSet *apps.StatefulSet, newSet *apps.StatefulSet) error {
	setReplicasAndDeleteSlots(newSet, *oldSet.Spec.Replicas-1, nil)
	return nil
}
