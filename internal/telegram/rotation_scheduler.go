package telegram

import (
	"context"
	"log"
	"time"
)

// startRotationScheduler keeps the materialized rotation schedule filled. It
// generates on startup and again every day just after midnight UTC. Generation
// always keeps the current week and next week materialized, so each Monday the
// pre-computed next-week turn becomes the active one — this is the weekly
// rotation. Past turns are never touched, preserving history.
func (h *Handler) startRotationScheduler(ctx context.Context) {
	if err := h.storage.GenerateAllRotations(ctx, time.Now()); err != nil {
		log.Printf("⚠️  rotation: initial generation failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(nextMidnightUTC())):
		}
		if err := h.storage.GenerateAllRotations(ctx, time.Now()); err != nil {
			log.Printf("⚠️  rotation: scheduled generation failed: %v", err)
		}
	}
}
