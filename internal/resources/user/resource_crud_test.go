// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package user

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// ---------- mock factory + connector ---------------------------------------

type mockFactory struct {
	get          func(server, db string) (database.DatabaseConnector, error)
	called       int
	serverScoped bool
}

func (m *mockFactory) ServerScopedPrincipals() bool { return m.serverScoped }

func (m *mockFactory) GetConnector(s, d string) (database.DatabaseConnector, error) {
	m.called++
	return m.get(s, d)
}

type mockConn struct {
	getFn        func(ctx context.Context, name string) (*database.User, error)
	createFn     func(ctx context.Context, u *database.User) error
	updateFn     func(ctx context.Context, u *database.User) error
	deleteFn     func(ctx context.Context, name string) error
	closeCalled  bool
	createCalled int
	deleteCalled int
}

// No-op: the real connectors call this from inside GetUser, so the resource
// layer never invokes it directly.
func (m *mockConn) CheckReadAccess(_ context.Context, _ database.ReadScope) error { return nil }

func (m *mockConn) GetUser(ctx context.Context, name string) (*database.User, error) {
	if m.getFn != nil {
		return m.getFn(ctx, name)
	}
	return nil, nil
}

func (m *mockConn) CreateUser(ctx context.Context, u *database.User) error {
	m.createCalled++
	if m.createFn != nil {
		return m.createFn(ctx, u)
	}
	return nil
}

func (m *mockConn) UpdateUser(ctx context.Context, u *database.User) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, u)
	}
	return nil
}

func (m *mockConn) DeleteUser(ctx context.Context, name string) error {
	m.deleteCalled++
	if m.deleteFn != nil {
		return m.deleteFn(ctx, name)
	}
	return nil
}

func (m *mockConn) GetRoleMember(_ context.Context, _, _ string) (*database.RoleMember, error) {
	return nil, nil
}
func (m *mockConn) CreateRoleMember(_ context.Context, _ *database.RoleMember) error { return nil }
func (m *mockConn) DeleteRoleMember(_ context.Context, _, _ string) error            { return nil }
func (m *mockConn) Close() error                                                     { m.closeCalled = true; return nil }

// fixedFactory builds a factory that always returns the same connector.
func fixedFactory(c database.DatabaseConnector) *mockFactory {
	return &mockFactory{get: func(string, string) (database.DatabaseConnector, error) { return c, nil }}
}

// errFactory builds a factory that always errors out of GetConnector.
func errFactory(err error) *mockFactory {
	return &mockFactory{get: func(string, string) (database.DatabaseConnector, error) { return nil, err }}
}

// planFromModel builds a tfsdk.Plan with Raw populated from the given model,
// suitable for passing as req.Plan to a CRUD method.
func planFromModel(ctx context.Context, t *testing.T, m userModel) tfsdk.Plan {
	t.Helper()
	state := emptyState(ctx, t, userSchema(t))
	if diags := state.Set(ctx, &m); diags.HasError() {
		t.Fatalf("State.Set failed: %v", diags)
	}
	return tfsdk.Plan(state)
}

func stateFromModel(ctx context.Context, t *testing.T, m userModel) tfsdk.State {
	t.Helper()
	state := emptyState(ctx, t, userSchema(t))
	if diags := state.Set(ctx, &m); diags.HasError() {
		t.Fatalf("State.Set failed: %v", diags)
	}
	return state
}

// ---------- Configure -------------------------------------------------------

func TestUserResource_Configure_Nil(t *testing.T) {
	r := &UserResource{}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("nil ProviderData should be a no-op, got %v", resp.Diagnostics)
	}
	if r.factory != nil {
		t.Fatalf("factory should remain nil after early-return")
	}
}

func TestUserResource_Configure_WrongType(t *testing.T) {
	r := &UserResource{}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "not-a-factory"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("wrong ProviderData type should produce an error diagnostic")
	}
}

func TestUserResource_Configure_OK(t *testing.T) {
	r := &UserResource{}
	f := &mockFactory{}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: f}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if r.factory == nil {
		t.Fatalf("factory should be stored")
	}
}

// ---------- Create ----------------------------------------------------------

func TestUserResource_Create_UserType_AssemblesID(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{
		createFn: func(_ context.Context, u *database.User) error {
			u.PrincipalID = 42
			u.DefaultSchema = "dbo"
			return nil
		},
	}
	r := &UserResource{factory: fixedFactory(conn)}

	plan := planFromModel(ctx, t, userModel{
		Server:   types.StringValue("myserver.database.windows.net"),
		Database: types.StringValue("mydb"),
		Type:     types.StringValue("user"),
		Name:     types.StringValue("juan.perez@milanesa.com"),
	})

	resp := &resource.CreateResponse{State: emptyState(ctx, t, userSchema(t))}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var got string
	resp.State.GetAttribute(ctx, path.Root("id"), &got)
	want := "myserver.database.windows.net/mydb/user/juan.perez@milanesa.com"
	if got != want {
		t.Fatalf("id = %q, want %q", got, want)
	}
	var schema string
	resp.State.GetAttribute(ctx, path.Root("default_schema"), &schema)
	if schema != "dbo" {
		t.Fatalf("default_schema = %q, want dbo", schema)
	}
	if !conn.closeCalled {
		t.Fatalf("conn.Close should be called")
	}
	if conn.createCalled != 1 {
		t.Fatalf("CreateUser called %d times, want 1", conn.createCalled)
	}
}

func TestUserResource_Create_GroupType_IDIncludesObjectID(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{}
	r := &UserResource{factory: fixedFactory(conn)}

	plan := planFromModel(ctx, t, userModel{
		Server:   types.StringValue("myserver.database.windows.net"),
		Database: types.StringValue("mydb"),
		Type:     types.StringValue("group"),
		Name:     types.StringValue("db.reader"),
		ObjectID: types.StringValue("00000000-0000-0000-0000-000000000000"),
	})

	resp := &resource.CreateResponse{State: emptyState(ctx, t, userSchema(t))}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var got string
	resp.State.GetAttribute(ctx, path.Root("id"), &got)
	want := "myserver.database.windows.net/mydb/group/db.reader/00000000-0000-0000-0000-000000000000"
	if got != want {
		t.Fatalf("id = %q, want %q", got, want)
	}
}

func TestUserResource_Create_FactoryError(t *testing.T) {
	ctx := context.Background()
	r := &UserResource{factory: errFactory(errors.New("boom"))}
	plan := planFromModel(ctx, t, userModel{
		Server: types.StringValue("myserver.database.windows.net"), Database: types.StringValue("mydb"),
		Type: types.StringValue("user"), Name: types.StringValue("juan.perez@milanesa.com"),
	})
	resp := &resource.CreateResponse{State: emptyState(ctx, t, userSchema(t))}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected error from factory")
	}
}

func TestUserResource_Create_ConnectorError(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{createFn: func(_ context.Context, _ *database.User) error { return errors.New("create-failed") }}
	r := &UserResource{factory: fixedFactory(conn)}
	plan := planFromModel(ctx, t, userModel{
		Server: types.StringValue("myserver.database.windows.net"), Database: types.StringValue("mydb"),
		Type: types.StringValue("user"), Name: types.StringValue("juan.perez@milanesa.com"),
	})
	resp := &resource.CreateResponse{State: emptyState(ctx, t, userSchema(t))}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected error from connector")
	}
	joined := ""
	for _, d := range resp.Diagnostics.Errors() {
		joined += d.Detail()
	}
	if !strings.Contains(joined, "create-failed") {
		t.Fatalf("error should bubble underlying message; got %q", joined)
	}
}

// ---------- Read ------------------------------------------------------------

func TestUserResource_Read_Success(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{
		getFn: func(_ context.Context, name string) (*database.User, error) {
			return &database.User{Name: name, PrincipalID: 7, DefaultSchema: "dbo"}, nil
		},
	}
	r := &UserResource{factory: fixedFactory(conn)}
	state := stateFromModel(ctx, t, userModel{
		ID:       types.StringValue("myserver.database.windows.net/mydb/user/juan.perez@milanesa.com"),
		Server:   types.StringValue("myserver.database.windows.net"),
		Database: types.StringValue("mydb"),
		Type:     types.StringValue("user"),
		Name:     types.StringValue("juan.perez@milanesa.com"),
	})
	resp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected: %v", resp.Diagnostics)
	}
	var got int64
	resp.State.GetAttribute(ctx, path.Root("principal_id"), &got)
	if got != 7 {
		t.Fatalf("principal_id = %d, want 7", got)
	}
}

func TestUserResource_Read_NotFound_RemovesResource(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{getFn: func(context.Context, string) (*database.User, error) { return nil, nil }}
	r := &UserResource{factory: fixedFactory(conn)}
	state := stateFromModel(ctx, t, userModel{
		Server: types.StringValue("myserver.database.windows.net"), Database: types.StringValue("mydb"),
		Type: types.StringValue("user"), Name: types.StringValue("juan.perez@milanesa.com"),
	})
	resp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected: %v", resp.Diagnostics)
	}
	// Resource removal: Raw should be null
	if !resp.State.Raw.IsNull() {
		t.Fatalf("Raw should be null after RemoveResource; got %v", resp.State.Raw)
	}
}

func TestUserResource_Read_FactoryError(t *testing.T) {
	ctx := context.Background()
	r := &UserResource{factory: errFactory(errors.New("boom"))}
	state := stateFromModel(ctx, t, userModel{
		Server: types.StringValue("myserver.database.windows.net"), Database: types.StringValue("mydb"),
		Type: types.StringValue("user"), Name: types.StringValue("juan.perez@milanesa.com"),
	})
	resp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected error from factory")
	}
}

func TestUserResource_Read_GetUserError(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{getFn: func(context.Context, string) (*database.User, error) { return nil, errors.New("read-failed") }}
	r := &UserResource{factory: fixedFactory(conn)}
	state := stateFromModel(ctx, t, userModel{
		Server: types.StringValue("myserver.database.windows.net"), Database: types.StringValue("mydb"),
		Type: types.StringValue("user"), Name: types.StringValue("juan.perez@milanesa.com"),
	})
	resp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected GetUser error to surface")
	}
}

// ---------- Update ----------------------------------------------------------

func TestUserResource_Update_Success(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{
		updateFn: func(_ context.Context, u *database.User) error {
			u.DefaultSchema = "myschema"
			return nil
		},
	}
	r := &UserResource{factory: fixedFactory(conn)}
	oldState := stateFromModel(ctx, t, userModel{
		Server: types.StringValue("myserver.database.windows.net"), Database: types.StringValue("mydb"),
		Type: types.StringValue("user"), Name: types.StringValue("juan.perez@milanesa.com"),
		DefaultSchema: types.StringValue("dbo"),
	})
	plan := planFromModel(ctx, t, userModel{
		Server: types.StringValue("myserver.database.windows.net"), Database: types.StringValue("mydb"),
		Type: types.StringValue("user"), Name: types.StringValue("juan.perez@milanesa.com"),
		DefaultSchema: types.StringValue("myschema"),
	})
	resp := &resource.UpdateResponse{State: emptyState(ctx, t, userSchema(t))}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: oldState}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected: %v", resp.Diagnostics)
	}
}

func TestUserResource_Update_FactoryError(t *testing.T) {
	ctx := context.Background()
	r := &UserResource{factory: errFactory(errors.New("boom"))}
	oldState := stateFromModel(ctx, t, userModel{
		Server: types.StringValue("myserver.database.windows.net"), Database: types.StringValue("mydb"),
		Type: types.StringValue("user"), Name: types.StringValue("juan.perez@milanesa.com"),
		DefaultSchema: types.StringValue("dbo"),
	})
	plan := planFromModel(ctx, t, userModel{
		Server: types.StringValue("myserver.database.windows.net"), Database: types.StringValue("mydb"),
		Type: types.StringValue("user"), Name: types.StringValue("juan.perez@milanesa.com"),
		DefaultSchema: types.StringValue("myschema"),
	})
	resp := &resource.UpdateResponse{State: emptyState(ctx, t, userSchema(t))}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: oldState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected error from factory")
	}
}

func TestUserResource_Update_ConnectorError(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{updateFn: func(context.Context, *database.User) error { return errors.New("update-failed") }}
	r := &UserResource{factory: fixedFactory(conn)}
	oldState := stateFromModel(ctx, t, userModel{
		Server: types.StringValue("myserver.database.windows.net"), Database: types.StringValue("mydb"),
		Type: types.StringValue("user"), Name: types.StringValue("juan.perez@milanesa.com"),
		DefaultSchema: types.StringValue("dbo"),
	})
	plan := planFromModel(ctx, t, userModel{
		Server: types.StringValue("myserver.database.windows.net"), Database: types.StringValue("mydb"),
		Type: types.StringValue("user"), Name: types.StringValue("juan.perez@milanesa.com"),
		DefaultSchema: types.StringValue("myschema"),
	})
	resp := &resource.UpdateResponse{State: emptyState(ctx, t, userSchema(t))}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: oldState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected error from connector")
	}
}

// ---------- Delete ----------------------------------------------------------

func TestUserResource_Delete_Success(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{}
	r := &UserResource{factory: fixedFactory(conn)}
	state := stateFromModel(ctx, t, userModel{
		Server: types.StringValue("myserver.database.windows.net"), Database: types.StringValue("mydb"),
		Type: types.StringValue("user"), Name: types.StringValue("juan.perez@milanesa.com"),
	})
	resp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected: %v", resp.Diagnostics)
	}
	if conn.deleteCalled != 1 {
		t.Fatalf("DeleteUser called %d times, want 1", conn.deleteCalled)
	}
}

func TestUserResource_Delete_FactoryError(t *testing.T) {
	ctx := context.Background()
	r := &UserResource{factory: errFactory(errors.New("boom"))}
	state := stateFromModel(ctx, t, userModel{
		Server: types.StringValue("myserver.database.windows.net"), Database: types.StringValue("mydb"),
		Type: types.StringValue("user"), Name: types.StringValue("juan.perez@milanesa.com"),
	})
	resp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected error from factory")
	}
}

func TestUserResource_Delete_ConnectorError(t *testing.T) {
	ctx := context.Background()
	conn := &mockConn{deleteFn: func(context.Context, string) error { return errors.New("delete-failed") }}
	r := &UserResource{factory: fixedFactory(conn)}
	state := stateFromModel(ctx, t, userModel{
		Server: types.StringValue("myserver.database.windows.net"), Database: types.StringValue("mydb"),
		Type: types.StringValue("user"), Name: types.StringValue("juan.perez@milanesa.com"),
	})
	resp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected error from connector")
	}
}

// ---------- ModifyPlan / database change -------------------------------------

func modifyPlan(t *testing.T, f database.ConnectorFactory, oldDB, newDB string) *resource.ModifyPlanResponse {
	t.Helper()
	ctx := context.Background()
	base := userModel{
		ID:     stringValue("myserver.postgres.database.azure.com/" + oldDB + "/group/db.reader/00000000-0000-0000-0000-000000000000"),
		Server: stringValue("myserver.postgres.database.azure.com"), Database: stringValue(oldDB),
		Type: stringValue("group"), Name: stringValue("db.reader"),
		ObjectID:      stringValue("00000000-0000-0000-0000-000000000000"),
		DefaultSchema: stringValue("public"), PrincipalID: types.Int64Value(1),
	}
	next := base
	next.Database = stringValue(newDB)
	plan := planFromModel(ctx, t, next)
	resp := &resource.ModifyPlanResponse{Plan: plan}
	r := &UserResource{factory: f}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: stateFromModel(ctx, t, base), Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan: %v", resp.Diagnostics)
	}
	return resp
}

func TestUserResource_ModifyPlan_DatabaseChange_ServerScoped_InPlace(t *testing.T) {
	resp := modifyPlan(t, &mockFactory{serverScoped: true}, "mydb", "postgres")
	if len(resp.RequiresReplace) != 0 {
		t.Fatalf("RequiresReplace = %v, want none", resp.RequiresReplace)
	}
	var id types.String
	resp.Diagnostics.Append(resp.Plan.GetAttribute(context.Background(), path.Root("id"), &id)...)
	if !id.IsUnknown() {
		t.Fatalf("planned id = %v, want unknown", id)
	}
}

func TestUserResource_ModifyPlan_DatabaseChange_SQLServer_RequiresReplace(t *testing.T) {
	resp := modifyPlan(t, &mockFactory{serverScoped: false}, "mydb", "otherdb")
	if !resp.RequiresReplace.Contains(path.Root("database")) {
		t.Fatalf("RequiresReplace = %v, want database", resp.RequiresReplace)
	}
}

func TestUserResource_ModifyPlan_SameDatabase_NoChange(t *testing.T) {
	resp := modifyPlan(t, &mockFactory{serverScoped: false}, "mydb", "mydb")
	if len(resp.RequiresReplace) != 0 {
		t.Fatalf("RequiresReplace = %v, want none", resp.RequiresReplace)
	}
}

func pgUserModels(oldSchema, newSchema string) (userModel, userModel) {
	mk := func(db, schema string, id types.String) userModel {
		return userModel{
			ID:     id,
			Server: stringValue("myserver.postgres.database.azure.com"), Database: stringValue(db),
			Type: stringValue("group"), Name: stringValue("db.reader"),
			ObjectID:      stringValue("00000000-0000-0000-0000-000000000000"),
			DefaultSchema: stringValue(schema), PrincipalID: types.Int64Value(1),
		}
	}
	return mk("mydb", oldSchema, stringValue("old")), mk("postgres", newSchema, types.StringUnknown())
}

func TestUserResource_Update_DatabaseOnly_ReadsThroughNewDatabaseWithoutAlter(t *testing.T) {
	ctx := context.Background()
	var gotDB string
	var gets, updates int
	conn := &mockConn{
		getFn: func(_ context.Context, name string) (*database.User, error) {
			gets++
			return &database.User{Name: name}, nil
		},
		updateFn: func(context.Context, *database.User) error { updates++; return nil },
	}
	f := &mockFactory{get: func(_, d string) (database.DatabaseConnector, error) { gotDB = d; return conn, nil }}
	oldM, newM := pgUserModels("public", "public")
	resp := &resource.UpdateResponse{State: emptyState(ctx, t, userSchema(t))}
	(&UserResource{factory: f}).Update(ctx, resource.UpdateRequest{Plan: planFromModel(ctx, t, newM), State: stateFromModel(ctx, t, oldM)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", resp.Diagnostics)
	}
	if gotDB != "postgres" || gets != 1 || updates != 0 {
		t.Fatalf("db=%q gets=%d updates=%d, want postgres/1/0", gotDB, gets, updates)
	}
	var got userModel
	resp.State.Get(ctx, &got)
	want := "myserver.postgres.database.azure.com/postgres/group/db.reader/00000000-0000-0000-0000-000000000000"
	if got.ID.ValueString() != want {
		t.Fatalf("id = %q, want %q", got.ID.ValueString(), want)
	}
	if got.DefaultSchema.ValueString() != "public" || got.PrincipalID.ValueInt64() != 1 {
		t.Fatalf("default_schema/principal_id not kept: %v", got)
	}
}

func TestUserResource_Update_DefaultSchemaChange_CallsUpdateUser(t *testing.T) {
	ctx := context.Background()
	var updates int
	conn := &mockConn{updateFn: func(context.Context, *database.User) error { updates++; return nil }}
	oldM, newM := pgUserModels("public", "myschema")
	resp := &resource.UpdateResponse{State: emptyState(ctx, t, userSchema(t))}
	(&UserResource{factory: fixedFactory(conn)}).Update(ctx, resource.UpdateRequest{Plan: planFromModel(ctx, t, newM), State: stateFromModel(ctx, t, oldM)}, resp)
	if resp.Diagnostics.HasError() || updates != 1 {
		t.Fatalf("updates=%d diags=%v, want 1 and none", updates, resp.Diagnostics)
	}
}

func TestUserResource_Update_DatabaseOnly_MissingUser_Errors(t *testing.T) {
	ctx := context.Background()
	oldM, newM := pgUserModels("public", "public")
	resp := &resource.UpdateResponse{State: emptyState(ctx, t, userSchema(t))}
	(&UserResource{factory: fixedFactory(&mockConn{})}).Update(ctx, resource.UpdateRequest{Plan: planFromModel(ctx, t, newM), State: stateFromModel(ctx, t, oldM)}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("want an error when the user is not visible from the new database")
	}
}
