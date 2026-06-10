// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package user

import "github.com/hashicorp/terraform-plugin-framework/types"

// stringValue is a one-line types.StringValue convenience used by table tests.
// Local helper — keeping it package-private avoids leaking test-only helpers.
func stringValue(s string) types.String { return types.StringValue(s) }
