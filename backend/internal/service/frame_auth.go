package service

import (
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/photoframe"
	"gorm.io/gorm"
)

// frameAuthObserver returns the status observer that keeps a device's
// auth_required flag in step with what its frame answers to the stored
// password. It runs inside each request, so under the same hold on the
// stored password as the request itself: a password change cannot slip in
// between the frame's answer and the flag it sets, and a flag set by a
// request made with the old password cannot land after the change stored
// the new one.
func frameAuthObserver(db *gorm.DB, id uint) func(photoframe.Answer) {
	return func(a photoframe.Answer) { recordFrameAuth(db, id, a) }
}

// frameAuthClock orders a device's answers for the observer. Two requests
// can be out at once, and the observer of the one answered first may run
// after the other's, so each decision is made under mu, from when the two
// requests went out and their answers came in: a refusal stands unless a
// request sent after it came in was accepted, and an acceptance clears the
// flag only if its request was sent after the latest refusal came in.
// Whichever observer runs first, the answer to the later-sent request wins.
// In memory only: after a restart every answer is newer than any before it.
type frameAuthClock struct {
	mu             sync.Mutex
	refusedAt      time.Time // when the latest 401 came in
	acceptedSentAt time.Time // when the latest accepted request went out
}

var frameAuthClocks sync.Map // uint -> *frameAuthClock

func frameAuthClockFor(id uint) *frameAuthClock {
	c, _ := frameAuthClocks.LoadOrStore(id, &frameAuthClock{})
	return c.(*frameAuthClock)
}

// recordFrameAuth records what the frame answered to a request made with the
// stored password. A 401 means the stored password (or the lack of one) is
// refused: auth_required is set, with auth_failed_at, once, at the first
// refusal, so it dates the run of refusals rather than the latest one. Any
// other answer went through the frame's password gate, so it clears the
// flag. Both subject to the device's frameAuthClock. A 429 says nothing
// about the password: the frame did not check it, and the lockout is per
// client address, so another client on this address may have caused it.
// Nothing changes on a 429.
func recordFrameAuth(db *gorm.DB, id uint, a photoframe.Answer) {
	clock := frameAuthClockFor(id)
	clock.mu.Lock()
	defer clock.mu.Unlock()
	switch a.Status {
	case http.StatusUnauthorized:
		if a.ReceivedAt.After(clock.refusedAt) {
			clock.refusedAt = a.ReceivedAt
		}
		if clock.acceptedSentAt.After(a.ReceivedAt) {
			// A request sent after this refusal came in was accepted: the
			// frame has changed its mind since, and that answer stands.
			return
		}
		res := db.Model(&model.Device{}).
			Where("id = ? AND auth_required = ?", id, false).
			Updates(map[string]interface{}{"auth_required": true, "auth_failed_at": time.Now()})
		if res.Error != nil {
			log.Printf("Failed to record that device %d refused the stored password: %v", id, res.Error)
		} else if res.RowsAffected > 0 {
			log.Printf("Device %d now refuses the stored password; its password must be entered again", id)
		}
	case http.StatusTooManyRequests:
	default:
		if a.SentAt.After(clock.acceptedSentAt) {
			clock.acceptedSentAt = a.SentAt
		}
		if !a.SentAt.After(clock.refusedAt) {
			// Out before the latest refusal came in, so possibly gated
			// before it: no proof the frame accepts the password now. The
			// next request, sent after the refusal, settles it.
			return
		}
		clearFrameAuth(db, id)
	}
}

// clearFrameAuth records that the frame accepts what the server sends it.
func clearFrameAuth(db *gorm.DB, id uint) {
	res := db.Model(&model.Device{}).
		Where("id = ? AND auth_required = ?", id, true).
		Updates(map[string]interface{}{"auth_required": false, "auth_failed_at": nil})
	if res.Error != nil {
		log.Printf("Failed to clear the password flag of device %d: %v", id, res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("Device %d accepts the stored password again", id)
	}
}

// frameStatus returns the status the frame answered with, or 0 when err is
// not an answer from the frame (unreachable, or not a frame error at all).
func frameStatus(err error) int {
	var se *photoframe.StatusError
	if errors.As(err, &se) {
		return se.StatusCode
	}
	return 0
}
