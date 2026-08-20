package service

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// failingClient is a mock client that can return errors on specified operations
type failingClient struct {
	client.Client
	listErr   error
	getErr    error
	createErr error
	updateErr error
	deleteErr error
	patchErr  error
	statusErr error // Status().Update() error
}

func newFailingClient(scheme *runtime.Scheme, objects ...client.Object) *failingClient {
	return &failingClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
	}
}

func (f *failingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if f.listErr != nil {
		return f.listErr
	}
	return f.Client.List(ctx, list, opts...)
}

func (f *failingClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if f.getErr != nil {
		return f.getErr
	}
	return f.Client.Get(ctx, key, obj, opts...)
}

func (f *failingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if f.createErr != nil {
		return f.createErr
	}
	return f.Client.Create(ctx, obj, opts...)
}

func (f *failingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	return f.Client.Update(ctx, obj, opts...)
}

func (f *failingClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return f.Client.Delete(ctx, obj, opts...)
}

func (f *failingClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if f.patchErr != nil {
		return f.patchErr
	}
	return f.Client.Patch(ctx, obj, patch, opts...)
}

type failingStatusClient struct {
	client.StatusWriter
	err error
}

func (s *failingStatusClient) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if s.err != nil {
		return s.err
	}
	return s.StatusWriter.Update(ctx, obj, opts...)
}

func (f *failingClient) Status() client.StatusWriter {
	if f.statusErr != nil {
		return &failingStatusClient{StatusWriter: f.Client.Status(), err: f.statusErr}
	}
	return f.Client.Status()
}

// helper: construct various K8s API errors
func newK8sNotFound(resource, name string) error {
	return apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: resource}, name)
}

func newK8sConflict(resource, name string) error {
	return apierrors.NewConflict(schema.GroupResource{Group: "", Resource: resource}, name, nil)
}

func newK8sAlreadyExists(resource, name string) error {
	return apierrors.NewAlreadyExists(schema.GroupResource{Group: "", Resource: resource}, name)
}

func newK8sForbidden(resource, name string) error {
	return apierrors.NewForbidden(schema.GroupResource{Group: "", Resource: resource}, name, nil)
}

func newK8sInternalError(msg string) error {
	return apierrors.NewInternalError(fmt.Errorf("%s", msg))
}
