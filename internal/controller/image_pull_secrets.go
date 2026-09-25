package controller

import (
	"context"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"
)

func resolveImagePullSecrets(
	ctx context.Context,
	reader client.Reader,
	discoveredImages []discovery.DiscoveredImage,
) ([]discovery.DiscoveredImage, error) {
	resolvedImages := slices.Clone(discoveredImages)

	serviceAccountCache := make(map[types.NamespacedName][]corev1.LocalObjectReference)

	for i := range resolvedImages {
		if len(resolvedImages[i].ImagePullSecrets) > 0 {
			continue
		}

		serviceAccountKey := types.NamespacedName{
			Namespace: resolvedImages[i].Namespace,
			Name:      resolvedImages[i].ServiceAccountName,
		}

		cachedSecrets, found := serviceAccountCache[serviceAccountKey]
		if found {
			resolvedImages[i].ImagePullSecrets = slices.Clone(cachedSecrets)
			continue
		}

		var serviceAccount corev1.ServiceAccount
		err := reader.Get(
			ctx,
			serviceAccountKey,
			&serviceAccount,
		)
		if err != nil {
			return nil, err
		}

		serviceAccountCache[serviceAccountKey] = slices.Clone(serviceAccount.ImagePullSecrets)
		resolvedImages[i].ImagePullSecrets = slices.Clone(serviceAccount.ImagePullSecrets)
	}

	return resolvedImages, nil
}
