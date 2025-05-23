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

package features

import (
	"flag"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

var (
	allFeatures     = sets.NewString(StableScheduling)
	defaultFeatures = map[string]bool{
		StableScheduling:    true,
		AdvancedStatefulSet: false,
		VolumeModifying:     false,
		VolumeReplacing:     false,
	}
	// DefaultFeatureGate is a shared global FeatureGate.
	DefaultFeatureGate FeatureGate = NewDefaultFeatureGate()
)

const (
	// StableScheduling controls stable scheduling of TiDB members.
	StableScheduling string = "StableScheduling"

	// AdvancedStatefulSet controls whether to use AdvancedStatefulSet to manage pods
	AdvancedStatefulSet string = "AdvancedStatefulSet"

	// VolumeModifying controls whether allow to modify volumes
	// NOTE: volume resize is always allowed even if this feature is disabled
	VolumeModifying string = "VolumeModifying"

	// VolumeReplacing controls whether to replace whole volumes by deleting and recreating on changes.
	// tidb, tikv & pd supported. If enabled takes precedence over resizing/modifying.
	VolumeReplacing string = "VolumeReplacing"
)

type FeatureGate interface {
	// AddFlag adds a flag for setting global feature gates to the specified FlagSet.
	AddFlag(flagset *flag.FlagSet)
	// Enabled returns true if the key is enabled.
	Enabled(key string) bool
	// Set parses and stores flag gates for known features
	// from a string like feature1=true,feature2=false,...
	Set(value string) error
	// SetFromMap stores flag gates for enabled features from a map[string]bool
	SetFromMap(m map[string]bool)
	// String returns a string representation of feature gate.
	String() string
}

var _ flag.Value = &featureGate{}

type featureGate struct {
	lock            sync.Mutex
	enabledFeatures map[string]bool
}

func (f *featureGate) AddFlag(flagset *flag.FlagSet) {
	flag.Var(f, "features", fmt.Sprintf("A set of key={true,false} pairs to enable/disable features, available features: %s", strings.Join(allFeatures.List(), ",")))
}

func (f *featureGate) Enabled(key string) bool {
	if b, ok := f.enabledFeatures[key]; ok {
		return b
	}
	return false
}

// String returns a string containing all enabled feature gates, formatted as "key1=value1,key2=value2,...".
func (f *featureGate) String() string {
	pairs := []string{}
	for k, v := range f.enabledFeatures {
		pairs = append(pairs, fmt.Sprintf("%s=%t", k, v))
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

func (f *featureGate) Set(value string) error {
	m := make(map[string]bool)
	for _, s := range strings.Split(value, ",") {
		if len(s) == 0 {
			continue
		}
		arr := strings.SplitN(s, "=", 2)
		k := strings.TrimSpace(arr[0])
		if len(arr) != 2 {
			return fmt.Errorf("missing bool value for %s", k)
		}
		v := strings.TrimSpace(arr[1])
		boolValue, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid value of %s=%s, err: %v", k, v, err)
		}
		m[k] = boolValue
	}
	f.SetFromMap(m)
	return nil
}

func (f *featureGate) SetFromMap(m map[string]bool) {
	f.lock.Lock()
	defer f.lock.Unlock()

	for k, v := range m {
		f.enabledFeatures[k] = v
	}

	klog.V(1).Infof("feature gates: %v", f.enabledFeatures)
}

func NewFeatureGate() FeatureGate {
	f := &featureGate{
		enabledFeatures: make(map[string]bool),
	}
	return f
}

func NewDefaultFeatureGate() FeatureGate {
	f := NewFeatureGate()
	f.SetFromMap(defaultFeatures)
	return f
}
