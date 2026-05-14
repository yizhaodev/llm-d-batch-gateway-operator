package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StatusPatch builds a merge patch (RFC 7396) targeting the /status subresource.
//
// Optimistic concurrency: the resourceVersion supplied to NewStatusPatch is
// embedded in the patch payload under metadata.resourceVersion. The API server
// returns 409 Conflict if the stored resourceVersion differs (i.e. the object
// was modified between the caller's Get and this Apply call).
//
// Each call to Add sets one field inside the status object. Field names must
// match the JSON tags of the status struct (e.g. "conditions",
// "observedGeneration", "componentStatus").
//
// Typical usage:
//
//	err := NewStatusPatch(gw.ResourceVersion).
//	    Add("conditions", gw.Status.Conditions).
//	    Add("observedGeneration", gw.Generation).
//	    Apply(ctx, r, &gw)
type StatusPatch struct {
	resourceVersion string
	fields          map[string]any
}

// NewStatusPatch returns a StatusPatch that will enforce optimistic concurrency
// using the given resourceVersion when Apply is called.
func NewStatusPatch(resourceVersion string) *StatusPatch {
	return &StatusPatch{
		resourceVersion: resourceVersion,
		fields:          make(map[string]any),
	}
}

// Add sets a field inside the status object. name must be the JSON field name
// (matching the json struct tag), e.g. "conditions", "observedGeneration".
func (p *StatusPatch) Add(name string, value any) *StatusPatch {
	p.fields[name] = value
	return p
}

// Apply serialises the accumulated status fields as a merge patch and sends
// it to the /status subresource. The resourceVersion from NewStatusPatch is
// embedded in the patch so that the API server enforces optimistic concurrency,
// returning 409 Conflict if the object was modified since the caller's Get.
func (p *StatusPatch) Apply(ctx context.Context, c client.Client, obj client.Object) error {
	patch := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": p.resourceVersion,
		},
		"status": p.fields,
	}

	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshalling status patch: %w", err)
	}

	return c.Status().Patch(ctx, obj, client.RawPatch(types.MergePatchType, data))
}
