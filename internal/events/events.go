package events

import "github.com/lojf/nextgen/internal/models"

// OnPromotion is called after a registration is promoted from waitlist → confirmed.
// services will call this if it's set.
var OnPromotion func(reg models.Registration)
