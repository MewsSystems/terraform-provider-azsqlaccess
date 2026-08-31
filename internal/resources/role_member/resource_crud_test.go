// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package role_member

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// ---------- mocks -----------------------------------------------------------

type mockFactory struct {
	get func(server, db string) (database.DatabaseConnector, error)
}

func (m *mockFactory) GetConnector(s, d string) (database.DatabaseConnector, error) {
	return m.get(s, d)
}

type mockConn struct {
	getFn        func(ctx context.Context, role, member string) (*database.RoleMember, error)
	createFn     func(ctx context.Context, rm *database.RoleMember) error
	deleteFn     func(ctx context.Context, role, member string) error
	createCalled int
	deleteCalled int
}

// No-op: the real connectors call this from inside GetRoleMember, so the
// resource layer never invokes it directly.
func (m *mockConn) CheckReadAccess(_ context.Context, _ database.ReadScope) error { return nil }

func (m *mockConn) GetUser(_ context.Context, _ string) (*database.User, error) { return nil, nil }
func (m *mockConn) CreateUser(_ context.Context, _ *database.User) error        { return nil }
func (m *mockConn) UpdateUser(_ context.Context, _ *database.User) error        { return nil }
func (m *mockConn) DeleteUser(_ context.Context, _ string) error                { return nil }
func (m *mockConn) Close() error                                                { return nil }

func (m *mockConn) GetRoleMember(ctx context.Context, role, member string) (*database.RoleMember, error) {
	if m.getFn != nil {
		return m.getFn(ctx, role, member)
	}
	return &database.RoleMember{Role: role, Member: member}, nil
}
func (m *mockConn) CreateRoleMember(ctx context.Context, rm *database.RoleMember) error {
	m.createCalled++
	if m.createFn != nil {
		return m.createFn(ctx, rm)
	}
	return nil
}
func (m *mockConn) DeleteRoleMember(ctx context.Context, role, member string) error {
	m.deleteCalled++
	if m.deleteFn != nil {
		return m.deleteFn(ctx, role, member)
	}
	return nil
}

func fixedFactory(c database.DatabaseConnector) *mockFactory {
	return &mockFactory{get: func(string, string) (database.DatabaseConnector, error) { return c, nil }}
}
func errFactory(err error) *mockFactory {
	return &mockFactory{get: func(string, string) (database.DatabaseConnector, error) { return nil, err }}
}

func planFromModel(ctx context.Context, t *testing.T, m roleMemberModel) tfsdk.Plan {
	t.Helper()
	state := emptyState(ctx, t, roleMemberSchema(t))
	if diags := state.Set(ctx, &m); diags.HasError() {
		t.Fatalf("State.Set: %v", diags)
	}
	return tfsdk.Plan(state)
}

func stateFromModel(ctx context.Context, t *testing.T, m roleMemberModel) tfsdk.State {
	t.Helper()
	state := emptyState(ctx, t, roleMemberSchema(t))
	if diags := state.Set(ctx, &m); diags.HasError() {
		t.Fatalf("State.Set: %v", diags)
	}
	return state
}

func sampleModel() roleMemberModel {
	return roleMemberModel{
		Server:   types.StringValue("myserver.database.windows.net"),
		Database: types.StringValue("mydb"),
		Role:     types.StringValue("db_datareader"),
		Member:   types.StringValue("juan.perez@milanesa.com"),
	}
}

// ---------- Configure -------------------------------------------------------

func TestRoleMemberResource_Configure_Nil(t *testing.T) {
	r := &RoleMemberResource{}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("nil ProviderData should be a no-op")
	}
	if r.factory != nil {
		t.Fatalf("factory should remain nil")
	}
}

func TestRoleMemberResource_Configure_WrongType(t *testing.T) {
	r := &RoleMemberResource{}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("wrong-typed ProviderData should error")
	}
}

func TestRoleMemberResource_Configure_OK(t *testing.T) {
	r := &RoleMemberResource{}
	f := &mockFactory{}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: f}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected: %v", resp.Diagnostics)
	}
	if r.factory == nil {
		t.Fatalf("factory should be stored")
	}
}

// ---------- Create ----------------------------------------------------------

func TestRoleMemberResource_Create_Success(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{}
	r := &RoleMemberResource{factory: fixedFactory(conn)}
	resp := &resource.CreateResponse{State: emptyState(ctx, t, roleMemberSchema(t))}
	r.Create(ctx, resource.CreateRequest{Plan: planFromModel(ctx, t, sampleModel())}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected: %v", resp.Diagnostics)
	}

	var got string
	resp.State.GetAttribute(ctx, path.Root("id"), &got)
	want := "myserver.database.windows.net/mydb/db_datareader/juan.perez@milanesa.com"
	if got != want {
		t.Fatalf("id = %q, want %q", got, want)
	}
	if conn.createCalled != 1 {
		t.Fatalf("CreateRoleMember called %d times, want 1", conn.createCalled)
	}
}

func TestRoleMemberResource_Create_FactoryError(t *testing.T) {
	ctx := context.Background()
	r := &RoleMemberResource{factory: errFactory(errors.New("boom"))}
	resp := &resource.CreateResponse{State: emptyState(ctx, t, roleMemberSchema(t))}
	r.Create(ctx, resource.CreateRequest{Plan: planFromModel(ctx, t, sampleModel())}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected factory error")
	}
}

func TestRoleMemberResource_Create_ConnectorError(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{createFn: func(context.Context, *database.RoleMember) error { return errors.New("grant-failed") }}
	r := &RoleMemberResource{factory: fixedFactory(conn)}
	resp := &resource.CreateResponse{State: emptyState(ctx, t, roleMemberSchema(t))}
	r.Create(ctx, resource.CreateRequest{Plan: planFromModel(ctx, t, sampleModel())}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected connector error")
	}
}

// ---------- Read ------------------------------------------------------------

func TestRoleMemberResource_Read_Success(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{}
	r := &RoleMemberResource{factory: fixedFactory(conn)}
	state := stateFromModel(ctx, t, sampleModel())
	resp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected: %v", resp.Diagnostics)
	}
}

func TestRoleMemberResource_Read_NotFound_RemovesResource(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{getFn: func(_ context.Context, _, _ string) (*database.RoleMember, error) { return nil, nil }}
	r := &RoleMemberResource{factory: fixedFactory(conn)}
	state := stateFromModel(ctx, t, sampleModel())
	resp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatalf("Raw should be null after removal")
	}
}

func TestRoleMemberResource_Read_FactoryError(t *testing.T) {
	ctx := context.Background()
	r := &RoleMemberResource{factory: errFactory(errors.New("boom"))}
	state := stateFromModel(ctx, t, sampleModel())
	resp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected factory error")
	}
}

func TestRoleMemberResource_Read_GetError(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{getFn: func(_ context.Context, _, _ string) (*database.RoleMember, error) {
		return nil, errors.New("read-failed")
	}}
	r := &RoleMemberResource{factory: fixedFactory(conn)}
	state := stateFromModel(ctx, t, sampleModel())
	resp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected GetRoleMember error")
	}
}

// ---------- Delete ----------------------------------------------------------

func TestRoleMemberResource_Delete_Success(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{}
	r := &RoleMemberResource{factory: fixedFactory(conn)}
	state := stateFromModel(ctx, t, sampleModel())
	resp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected: %v", resp.Diagnostics)
	}
	if conn.deleteCalled != 1 {
		t.Fatalf("DeleteRoleMember called %d times, want 1", conn.deleteCalled)
	}
}

func TestRoleMemberResource_Delete_FactoryError(t *testing.T) {
	ctx := context.Background()
	r := &RoleMemberResource{factory: errFactory(errors.New("boom"))}
	state := stateFromModel(ctx, t, sampleModel())
	resp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected factory error")
	}
}

func TestRoleMemberResource_Delete_ConnectorError(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{deleteFn: func(_ context.Context, _, _ string) error { return errors.New("revoke-failed") }}
	r := &RoleMemberResource{factory: fixedFactory(conn)}
	state := stateFromModel(ctx, t, sampleModel())
	resp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected connector error")
	}
}
