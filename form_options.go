package main

func isApprovedOnly(form FormMapping) bool {
	return form.ApprovedOnly
}

func shouldApproveAfterSync(form FormMapping) bool {
	return form.ApproveAfterSync
}