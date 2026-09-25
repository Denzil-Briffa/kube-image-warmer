package controller

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"
)

const (
	testApplicationName          = "application"
	testServiceAccountName       = "registry-reader"
	testMissingServiceAccount    = "missing-service-account"
	testExplicitSecretName       = "explicit-registry-secret"
	testServiceAccountSecretName = "service-account-registry-secret"
)

type serviceAccountGetCountingReader struct {
	client.Reader
	getCalls int
}

func (r *serviceAccountGetCountingReader) Get(
	ctx context.Context,
	key client.ObjectKey,
	obj client.Object,
	opts ...client.GetOption,
) error {
	r.getCalls++

	return r.Reader.Get(ctx, key, obj, opts...)
}

func TestResolveImagePullSecretsKeepsExplicitSecrets(t *testing.T) {
	scheme := runtime.NewScheme()

	err := corev1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("failed to add core Kubernetes types to scheme: %v", err)
	}

	reader := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	discoveredImages := []discovery.DiscoveredImage{
		{
			Image:              "private.example.com/application:v1",
			Namespace:          testApplicationName,
			ServiceAccountName: testMissingServiceAccount,
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: testExplicitSecretName},
			},
		},
	}

	got, err := resolveImagePullSecrets(
		context.Background(),
		reader,
		discoveredImages,
	)
	if err != nil {
		t.Fatalf("resolveImagePullSecrets returned an unexpected error: %v", err)
	}

	if !reflect.DeepEqual(got, discoveredImages) {
		t.Fatalf(
			"expected explicit pull secrets to remain unchanged, got %#v",
			got,
		)
	}
}

func TestResolveImagePullSecretsUsesServiceAccount(t *testing.T) {
	scheme := runtime.NewScheme()

	err := corev1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("failed to add core Kubernetes types to scheme: %v", err)
	}

	serviceAccount := &corev1.ServiceAccount{}
	serviceAccount.Namespace = testApplicationName
	serviceAccount.Name = testServiceAccountName
	serviceAccount.ImagePullSecrets = []corev1.LocalObjectReference{
		{Name: testServiceAccountSecretName},
	}

	fakeReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(serviceAccount).
		Build()

	reader := &serviceAccountGetCountingReader{
		Reader: fakeReader,
	}

	discoveredImages := []discovery.DiscoveredImage{
		{
			Image:              "private.example.com/application:v1",
			Namespace:          testApplicationName,
			ServiceAccountName: testServiceAccountName,
		},
		{
			Image:              "private.example.com/worker:v1",
			Namespace:          testApplicationName,
			ServiceAccountName: testServiceAccountName,
		},
	}

	got, err := resolveImagePullSecrets(
		context.Background(),
		reader,
		discoveredImages,
	)
	if err != nil {
		t.Fatalf("resolveImagePullSecrets returned an unexpected error: %v", err)
	}

	expectedSecrets := serviceAccount.ImagePullSecrets

	for i := range got {
		if !reflect.DeepEqual(got[i].ImagePullSecrets, expectedSecrets) {
			t.Errorf(
				"image %q: expected pull secrets %#v, got %#v",
				got[i].Image,
				expectedSecrets,
				got[i].ImagePullSecrets,
			)
		}
	}

	if reader.getCalls != 1 {
		t.Errorf(
			"expected one ServiceAccount lookup, got %d",
			reader.getCalls,
		)
	}

	for i := range discoveredImages {
		if len(discoveredImages[i].ImagePullSecrets) != 0 {
			t.Errorf(
				"input image %q was modified",
				discoveredImages[i].Image,
			)
		}
	}
}

func TestResolveImagePullSecretsReturnsMissingServiceAccountError(
	t *testing.T,
) {
	scheme := runtime.NewScheme()

	err := corev1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("failed to add core Kubernetes types to scheme: %v", err)
	}

	reader := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	discoveredImages := []discovery.DiscoveredImage{
		{
			Image:              "private.example.com/missing:v1",
			Namespace:          testApplicationName,
			ServiceAccountName: testMissingServiceAccount,
		},
	}

	_, err = resolveImagePullSecrets(
		context.Background(),
		reader,
		discoveredImages,
	)
	if err == nil {
		t.Fatal("expected an error for a missing ServiceAccount")
	}

	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected a not-found error, got %v", err)
	}
}
