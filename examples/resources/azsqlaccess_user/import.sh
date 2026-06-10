# type = user — server/database/user/upn (no object_id)
terraform import azsqlaccess_user.user \
  "myserver.database.windows.net/mydb/user/juan.perez@milanesa.com"

# type = group — server/database/group/display-name/object-id
terraform import azsqlaccess_user.group \
  "myserver.database.windows.net/mydb/group/db.reader/00000000-0000-0000-0000-000000000000"

# type = service_principal — server/database/service_principal/display-name/object-id
terraform import azsqlaccess_user.managed_identity \
  "myserver.database.windows.net/mydb/service_principal/myapp-identity/00000000-0000-0000-0000-000000000000"
