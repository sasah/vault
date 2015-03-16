package vault

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
)

// logical.Factory
func PassthroughBackendFactory(map[string]string) (logical.Backend, error) {
	var b PassthroughBackend
	return &framework.Backend{
		Paths: []*framework.Path{
			&framework.Path{
				Pattern: ".*",
				Fields: map[string]*framework.FieldSchema{
					"lease": &framework.FieldSchema{
						Type:        framework.TypeString,
						Description: "Lease time for this key when read. Ex: 1h",
					},
				},

				Callbacks: map[logical.Operation]framework.OperationFunc{
					logical.ReadOperation:   b.handleRead,
					logical.RenewOperation:  b.handleRead,
					logical.WriteOperation:  b.handleWrite,
					logical.DeleteOperation: b.handleDelete,
					logical.ListOperation:   b.handleList,
				},

				HelpSynopsis:    strings.TrimSpace(passthroughHelpSynopsis),
				HelpDescription: strings.TrimSpace(passthroughHelpDescription),
			},
		},
	}, nil
}

// PassthroughBackend is used storing secrets directly into the physical
// backend. The secrest are encrypted in the durable storage and custom lease
// information can be specified, but otherwise this backend doesn't do anything
// fancy.
type PassthroughBackend struct{}

func (b *PassthroughBackend) handleRead(
	req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	// Read the path
	out, err := req.Storage.Get(req.Path)
	if err != nil {
		return nil, fmt.Errorf("read failed: %v", err)
	}

	// Fast-path the no data case
	if out == nil {
		return nil, nil
	}

	// Decode the data
	var raw map[string]interface{}
	if err := json.Unmarshal(out.Value, &raw); err != nil {
		return nil, fmt.Errorf("json decoding failed: %v", err)
	}

	// Check if there is a lease key
	leaseVal, ok := raw["lease"].(string)
	var lease *logical.Lease
	if ok {
		leaseDuration, err := time.ParseDuration(leaseVal)
		if err == nil {
			lease = &logical.Lease{
				Renewable: false,
				Duration:  leaseDuration,
			}
		}
	}

	// Generate the response
	resp := &logical.Response{
		IsSecret: true,
		Lease:    lease,
		Data:     raw,
	}
	return resp, nil
}

func (b *PassthroughBackend) handleWrite(
	req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	// Check that some fields are given
	if len(req.Data) == 0 {
		return nil, fmt.Errorf("missing data fields")
	}

	// JSON encode the data
	buf, err := json.Marshal(req.Data)
	if err != nil {
		return nil, fmt.Errorf("json encoding failed: %v", err)
	}

	// Write out a new key
	entry := &logical.StorageEntry{
		Key:   req.Path,
		Value: buf,
	}
	if err := req.Storage.Put(entry); err != nil {
		return nil, fmt.Errorf("failed to write: %v", err)
	}

	return nil, nil
}

func (b *PassthroughBackend) handleDelete(
	req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	// Delete the key at the request path
	if err := req.Storage.Delete(req.Path); err != nil {
		return nil, err
	}

	return nil, nil
}

func (b *PassthroughBackend) handleList(
	req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	// List the keys at the prefix given by the request
	keys, err := req.Storage.List(req.Path)
	if err != nil {
		return nil, err
	}

	// Generate the response
	return logical.ListResponse(keys), nil
}

const passthroughHelpSynopsis = `
Pass-through secret storage to the physical backend, allowing you to
read/write arbitrary data into secret storage.
`

const passthroughHelpDescription = `
The pass-through backend reads and writes arbitrary data into secret storage,
encrypting it along the way.

A lease can be specified when writing with the "lease" field. If given, then
when the secret is read, Vault will report a lease with that duration. It
is expected that the consumer of this backend properly writes renewed keys
before the lease is up. In addition, revocation must be handled by the
user of this backend.
`
