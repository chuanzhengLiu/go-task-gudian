package api

import (
	"testing"
	"time"

	"ancient-texts-backend/internal/model"
)

func TestLockHeldByOther(t *testing.T) {
	now := time.Now()
	owner := uint64(1)
	other := uint64(2)

	freshLock := now.Add(-1 * time.Minute)
	expiredLock := now.Add(-6 * time.Minute)
	justExpiredLock := now.Add(-5*time.Minute - time.Second)

	tests := []struct {
		name   string
		page   model.Page
		userID uint64
		want   bool
	}{
		{
			name:   "unlocked page can be locked by anyone",
			page:   model.Page{},
			userID: other,
			want:   false,
		},
		{
			name:   "own lock can be renewed",
			page:   model.Page{LockBy: &owner, LockAt: &freshLock},
			userID: owner,
			want:   false,
		},
		{
			name:   "fresh lock held by another user blocks takeover",
			page:   model.Page{LockBy: &owner, LockAt: &freshLock},
			userID: other,
			want:   true,
		},
		{
			name:   "lock without timestamp held by another user blocks takeover",
			page:   model.Page{LockBy: &owner},
			userID: other,
			want:   true,
		},
		{
			name:   "expired lock held by another user can be taken over",
			page:   model.Page{LockBy: &owner, LockAt: &expiredLock},
			userID: other,
			want:   false,
		},
		{
			name:   "lock just past 5 minutes is expired",
			page:   model.Page{LockBy: &owner, LockAt: &justExpiredLock},
			userID: other,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lockHeldByOther(&tt.page, tt.userID, now); got != tt.want {
				t.Errorf("lockHeldByOther() = %v, want %v", got, tt.want)
			}
		})
	}
}
