package handlers

import (
	"log"
	"net/http"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

// writeAudit records a mutating action. It never fails the request: an audit
// write that errors is logged and swallowed, because losing the trail is better
// than refusing to check a child in.
func writeAudit(r *http.Request, u *models.AdminUser, action, target, detail string) {
	entry := models.AuditLog{
		StaffedBy: StaffName(r),
		IP:        clientIP(r),
		Action:    action,
		Target:    target,
		Detail:    detail,
	}
	if u == nil {
		u = CurrentUser(r)
	}
	if u != nil {
		entry.Username = u.Username
		entry.Role = u.Role
	}
	if err := db.Conn().Create(&entry).Error; err != nil {
		log.Printf("audit write failed (%s %s): %v", action, target, err)
	}
}
