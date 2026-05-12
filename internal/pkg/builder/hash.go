/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package builder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
)

// ComputeConfigHash computes a SHA256 hash of all ConfigMaps and Secrets
// referenced by the WebApp spec (envFrom + volumes). Returns empty string
// if no external config is referenced or if any resource is not found.
//
// The hash is injected as a Deployment template annotation so that changes
// to ConfigMap/Secret data trigger a Pod rollout automatically.
func ComputeConfigHash(ctx context.Context, c client.Client, webapp *myappv1alpha1.WebApp) (string, error) {
	var parts []string

	// Collect ConfigMap data from envFrom
	for _, ef := range webapp.Spec.EnvFrom {
		if ef.ConfigMapRef != nil {
			cm := &corev1.ConfigMap{}
			if err := c.Get(ctx, types.NamespacedName{
				Name:      ef.ConfigMapRef.Name,
				Namespace: webapp.Namespace,
			}, cm); err != nil {
				return "", fmt.Errorf("envFrom configMap %s: %w", ef.ConfigMapRef.Name, err)
			}
			parts = append(parts, serializeData(cm.Data))
		}
		if ef.SecretRef != nil {
			sec := &corev1.Secret{}
			if err := c.Get(ctx, types.NamespacedName{
				Name:      ef.SecretRef.Name,
				Namespace: webapp.Namespace,
			}, sec); err != nil {
				return "", fmt.Errorf("envFrom secret %s: %w", ef.SecretRef.Name, err)
			}
			parts = append(parts, serializeSecretData(sec.Data))
		}
	}

	// Collect ConfigMap/Secret data from volumes
	for _, v := range webapp.Spec.Volumes {
		if v.ConfigMap != nil {
			cm := &corev1.ConfigMap{}
			if err := c.Get(ctx, types.NamespacedName{
				Name:      v.ConfigMap.Name,
				Namespace: webapp.Namespace,
			}, cm); err != nil {
				return "", fmt.Errorf("volume configMap %s: %w", v.ConfigMap.Name, err)
			}
			parts = append(parts, serializeData(cm.Data))
		}
		if v.Secret != nil {
			sec := &corev1.Secret{}
			if err := c.Get(ctx, types.NamespacedName{
				Name:      v.Secret.Name,
				Namespace: webapp.Namespace,
			}, sec); err != nil {
				return "", fmt.Errorf("volume secret %s: %w", v.Secret.Name, err)
			}
			parts = append(parts, serializeSecretData(sec.Data))
		}
	}

	if len(parts) == 0 {
		return "", nil
	}

	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h[:]), nil
}

func serializeData(data map[string]string) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(data[k])
		sb.WriteString(";")
	}
	return sb.String()
}

func serializeSecretData(data map[string][]byte) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(string(data[k]))
		sb.WriteString(";")
	}
	return sb.String()
}
