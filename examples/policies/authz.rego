package infractl.authz

default allow := false

# Allow all read operations.
allow if {
	input.action == "read"
}

# Allow create/update/delete only for admin users.
allow if {
	input.action != "read"
	input.subject.user == "admin"
}
