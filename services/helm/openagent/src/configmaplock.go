package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// ConfigMapLock implements resourcelock.Interface using a ConfigMap annotation.
// This is needed because client-go v0.29 removed the upstream ConfigMapLock struct.
type ConfigMapLock struct {
	ConfigMapMeta metav1.ObjectMeta
	Client        corev1client.ConfigMapsGetter
	LockConfig    resourcelock.ResourceLockConfig
	cm            *v1.ConfigMap
}

func (cml *ConfigMapLock) Get(ctx context.Context) (*resourcelock.LeaderElectionRecord, []byte, error) {
	var record resourcelock.LeaderElectionRecord
	var err error
	cml.cm, err = cml.Client.ConfigMaps(cml.ConfigMapMeta.Namespace).Get(ctx, cml.ConfigMapMeta.Name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}
	if cml.cm.Annotations == nil {
		cml.cm.Annotations = make(map[string]string)
	}
	if recordBytes, found := cml.cm.Annotations[resourcelock.LeaderElectionRecordAnnotationKey]; found {
		if err := json.Unmarshal([]byte(recordBytes), &record); err != nil {
			return nil, nil, err
		}
		return &record, []byte(recordBytes), nil
	}
	return &record, nil, nil
}

func (cml *ConfigMapLock) Create(ctx context.Context, ler resourcelock.LeaderElectionRecord) error {
	recordBytes, err := json.Marshal(ler)
	if err != nil {
		return err
	}
	cml.cm, err = cml.Client.ConfigMaps(cml.ConfigMapMeta.Namespace).Create(ctx, &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cml.ConfigMapMeta.Name,
			Namespace: cml.ConfigMapMeta.Namespace,
			Annotations: map[string]string{
				resourcelock.LeaderElectionRecordAnnotationKey: string(recordBytes),
			},
		},
	}, metav1.CreateOptions{})
	return err
}

func (cml *ConfigMapLock) Update(ctx context.Context, ler resourcelock.LeaderElectionRecord) error {
	if cml.cm == nil {
		return errors.New("configmap not initialized, call get or create first")
	}
	recordBytes, err := json.Marshal(ler)
	if err != nil {
		return err
	}
	cml.cm.Annotations[resourcelock.LeaderElectionRecordAnnotationKey] = string(recordBytes)
	cml.cm, err = cml.Client.ConfigMaps(cml.ConfigMapMeta.Namespace).Update(ctx, cml.cm, metav1.UpdateOptions{})
	return err
}

func (cml *ConfigMapLock) RecordEvent(s string) {
	if cml.LockConfig.EventRecorder == nil {
		return
	}
	events := fmt.Sprintf("%v %v", cml.LockConfig.Identity, s)
	subject := &v1.ConfigMap{ObjectMeta: cml.cm.ObjectMeta}
	subject.Kind = "ConfigMap"
	subject.APIVersion = "v1"
	cml.LockConfig.EventRecorder.Eventf(subject, v1.EventTypeNormal, "LeaderElection", events)
}

func (cml *ConfigMapLock) Describe() string {
	return fmt.Sprintf("%v/%v", cml.ConfigMapMeta.Namespace, cml.ConfigMapMeta.Name)
}

func (cml *ConfigMapLock) Identity() string {
	return cml.LockConfig.Identity
}
