package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/photoframe"
	"gorm.io/gorm"
)

// frameCredLocks holds one RWMutex per device id. ChangeFramePassword takes it
// exclusively for as long as the frame's password and the stored one can
// disagree; every other request to the frame that authenticates with the
// stored password takes it shared (see FrameClient). Without it, a config
// pull that loaded the old password just before a change keeps retrying with
// it, and each 401 spends one of the frame's free wrong guesses before it
// locks this server out.
//
// The writers of the stored password and the host (SetHTTPPassword, and
// UpdateDevice when it changes the host) take it exclusively too, since
// both reset the device's auth_required flag: the requests in flight must
// record the frame's answer to the old password or host first, or a late
// 401 lands on the new one.
var frameCredLocks sync.Map // uint -> *sync.RWMutex

func frameCredLock(id uint) *sync.RWMutex {
	l, _ := frameCredLocks.LoadOrStore(id, &sync.RWMutex{})
	return l.(*sync.RWMutex)
}

// holdFrameCreds keeps a frame password change, or a store of the password
// or host, from running until the returned release is called. Requests to
// the frame take it, and so does deleting the device: shared, since they
// only need to stay out of a change's way, not each other's. A change that
// ran alongside a delete would have nowhere to store its result.
func holdFrameCreds(id uint) (release func()) {
	l := frameCredLock(id)
	l.RLock()
	return l.RUnlock
}

// FrameClient returns a client for the device's frame, authenticated with the
// password stored for it right now, and holds that password in place until
// release is called: a password change waits for the release, so the client
// never goes out with a password the frame has just stopped accepting. Keep
// the hold to the requests themselves, since a change waits on it.
//
// Every answer the frame gives is recorded on the device's auth_required
// flag (see recordFrameAuth), so a frame that starts refusing the stored
// password is noticed by whichever request hits it first.
func FrameClient(db *gorm.DB, id uint) (client *photoframe.Client, release func(), err error) {
	release = holdFrameCreds(id)
	var device model.Device
	if err := db.Select("id", "host", "http_password").First(&device, id).Error; err != nil {
		release()
		return nil, nil, err
	}
	client = photoframe.NewClientWithPassword(device.Host, device.HTTPPassword).
		WithStatusObserver(frameAuthObserver(db, id))
	return client, release, nil
}

// FrameClientAt is FrameClient for a caller that has already committed to a
// host -- an image rendered for the frame there, or an edit loaded with the
// device. If the saved host has been
// edited meanwhile it refuses, rather than send the image to a frame it was
// not rendered for, or the old frame the new frame's password.
func FrameClientAt(db *gorm.DB, id uint, host string) (client *photoframe.Client, release func(), err error) {
	client, release, err = FrameClient(db, id)
	if err != nil {
		return nil, nil, err
	}
	if client.Host() != host {
		release()
		return nil, nil, fmt.Errorf("device host changed from %s to %s meanwhile", host, client.Host())
	}
	return client, release, nil
}

// IsFrameAuthError reports whether err is the frame refusing this server's
// password (401) or refusing to check it for now (429). Retrying either with
// the same password gains nothing and, for a 401, counts as another guess.
func IsFrameAuthError(err error) bool {
	status := frameStatus(err)
	return status == 401 || status == 429
}

// ErrFrameHostChanged: the device's saved host is not the one the caller
// expected to change the password on.
var ErrFrameHostChanged = errors.New("the device's host has changed")

// FramePasswordFailure says why a frame password change did not happen.
type FramePasswordFailure int

const (
	// FrameNoHost: the device has no host to reach it at.
	FrameNoHost FramePasswordFailure = iota
	// FrameUnreachable: the frame could not be asked.
	FrameUnreachable
	// FrameWrongPassword: the frame rejected the stored password (401).
	FrameWrongPassword
	// FrameLockedOut: the frame is refusing password attempts from this
	// server for a while after too many wrong ones (429).
	FrameLockedOut
	// FrameRefused: the frame answered, but did not take the new password.
	FrameRefused
	// FrameUnsupported: the frame's firmware predates frame passwords.
	FrameUnsupported
	// FrameOutcomeUnknown: the change request got no answer, and neither did
	// the check afterwards, so whether the frame took the new password is
	// unknown. The stored password is unchanged, but may no longer work.
	FrameOutcomeUnknown
	// FrameOpen: the frame does not require a password at the moment, so it
	// could not confirm the one given (it takes any). Only from
	// checkFramePassword.
	FrameOpen
)

// FramePasswordError is returned by ChangeFramePassword when the frame did
// not take the new password, or (FrameOutcomeUnknown) may not have. The stored
// password is unchanged whenever it is returned.
type FramePasswordError struct {
	Kind FramePasswordFailure
	// RetryAfter is the frame's Retry-After, in seconds, on FrameLockedOut.
	RetryAfter string
	// NoneStored, on FrameWrongPassword: the server had no password for the
	// frame at all, rather than a wrong one.
	NoneStored bool
	Err        error
}

func (e *FramePasswordError) Error() string {
	switch e.Kind {
	case FrameNoHost:
		return "the device has no host to reach the frame at"
	case FrameWrongPassword:
		return "the frame rejected the password stored for it"
	case FrameLockedOut:
		return "the frame is refusing password attempts after too many wrong ones"
	case FrameRefused:
		return fmt.Sprintf("the frame refused the new password: %v", e.Err)
	case FrameUnsupported:
		return "the frame's firmware does not support a password; update it first"
	case FrameOutcomeUnknown:
		return fmt.Sprintf("the frame did not answer, so whether it took the new password is unknown: %v", e.Err)
	case FrameOpen:
		return "the frame does not require a password at the moment"
	default:
		return fmt.Sprintf("could not reach the frame: %v", e.Err)
	}
}

func (e *FramePasswordError) Unwrap() error { return e.Err }

// FramePasswordStoreError means the frame took the new password but storing
// it here failed, so the server still holds the old one. Distinct from
// FramePasswordError because the frame HAS changed.
type FramePasswordStoreError struct{ Err error }

func (e *FramePasswordStoreError) Error() string {
	return fmt.Sprintf("the frame now uses the new password, but storing it failed: %v", e.Err)
}

func (e *FramePasswordStoreError) Unwrap() error { return e.Err }

// ChangeFramePassword changes the password on the frame itself, then stores
// it, so the server keeps authenticating after the change. "" turns the
// frame's password off.
//
// The frame accepts a new password only via an authenticated PATCH, made
// with the password stored now; it deliberately ignores one arriving in the
// server's config push, which is not authenticated. Only once the frame has
// taken it is the new one stored, so any failure before that leaves the
// server and the frame agreeing on the old one.
//
// verified reports whether a read-back with the new password showed the
// frame's authentication in the expected state. When the frame itself says it
// does not use the new password, the change is refused and nothing stored;
// when the read-back only goes unanswered after the frame accepted the PATCH,
// the new password is stored anyway (verified false), since the frame said
// it took it.
//
// expectedHost, when not empty, is the host the user saw: if the saved host
// differs (edited elsewhere meanwhile), nothing is sent, since the change
// would land on a different frame. Such a mismatch returns ErrFrameHostChanged.
func (s *DeviceService) ChangeFramePassword(id uint, newPassword, expectedHost string) (verified bool, err error) {
	if err := photoframe.ValidateHTTPPassword(newPassword); err != nil {
		return false, err
	}

	l := frameCredLock(id)
	l.Lock()
	defer l.Unlock()

	// Loaded under the lock, so no other change can have moved it since.
	var device model.Device
	if err := s.db.Select("id", "host", "http_password").First(&device, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrDeviceNotFound
		}
		return false, err
	}
	if device.Host == "" {
		return false, &FramePasswordError{Kind: FrameNoHost}
	}
	if expectedHost != "" && expectedHost != device.Host {
		return false, ErrFrameHostChanged
	}

	// The pre-check and the PATCH go out with the stored password, so their
	// answers say whether it still works: recorded like any other request's.
	client := photoframe.NewClientWithPassword(device.Host, device.HTTPPassword).
		WithStatusObserver(frameAuthObserver(s.db, id))

	// Read the config first, with the stored password: it shows that password
	// still works, so a wrong one fails here and not halfway, and that the
	// firmware knows about passwords at all. Older firmware ignores an
	// unknown http_password and answers 200, which would leave a password
	// stored for a frame that never took it.
	raw, err := client.FetchConfig()
	if err != nil {
		fpe := framePasswordError(err)
		fpe.NoneStored = fpe.Kind == FrameWrongPassword && device.HTTPPassword == ""
		return false, fpe
	}
	if _, err := httpAuthEnabled(raw); err != nil {
		return false, &FramePasswordError{Kind: FrameUnsupported, Err: err}
	}

	patchErr := client.PatchConfig(map[string]interface{}{"http_password": newPassword})
	var se *photoframe.StatusError
	if errors.As(patchErr, &se) {
		// The frame answered, so it did not change anything.
		return false, framePasswordError(patchErr)
	}

	// Read back with the new password: after an answered PATCH to confirm it,
	// after an unanswered one (the request may have been applied with only
	// the response lost) to find out whether it took.
	verified, verr := frameUsesPassword(device.Host, newPassword)
	switch {
	case verified:
		if patchErr != nil {
			log.Printf("Frame %s took the new password although the change request failed (%v)", device.Host, patchErr)
		}
	case isDefinitiveNo(verr):
		// The frame itself says it does not use the new password, whatever
		// the PATCH said; the old one is what it still takes.
		if patchErr != nil {
			return false, &FramePasswordError{Kind: FrameUnreachable, Err: patchErr}
		}
		return false, &FramePasswordError{Kind: FrameRefused, Err: fmt.Errorf("it answered the change, but does not use the new password (%v)", verr)}
	case patchErr != nil:
		// Neither answer came back: the frame may or may not have the new
		// password, and saying "nothing changed" could be wrong.
		return false, &FramePasswordError{Kind: FrameOutcomeUnknown, Err: patchErr}
	default:
		// The frame accepted the PATCH and only the read-back went
		// unanswered. Its 200 is the best word there is: follow it.
		log.Printf("Frame %s accepted the new password, but reading it back failed: %v", device.Host, verr)
	}

	// The frame took the new password, so whatever it refused before is
	// history: the flag goes with the old password.
	res := s.db.Model(&model.Device{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"http_password": newPassword, "auth_required": false, "auth_failed_at": nil,
		})
	if res.Error != nil {
		return false, &FramePasswordStoreError{Err: res.Error}
	}
	if res.RowsAffected == 0 {
		return false, &FramePasswordStoreError{Err: ErrDeviceNotFound}
	}
	return verified, nil
}

// framePasswordError classifies an error from a request to the frame made
// with the stored password.
func framePasswordError(err error) *FramePasswordError {
	var se *photoframe.StatusError
	if !errors.As(err, &se) {
		return &FramePasswordError{Kind: FrameUnreachable, Err: err}
	}
	switch se.StatusCode {
	case 401:
		return &FramePasswordError{Kind: FrameWrongPassword, Err: err}
	case 429:
		return &FramePasswordError{Kind: FrameLockedOut, RetryAfter: se.RetryAfter, Err: err}
	}
	if se.Message != "" {
		err = errors.New(se.Message)
	}
	return &FramePasswordError{Kind: FrameRefused, Err: err}
}

// isDefinitiveNo reports whether a failed frameUsesPassword check is the
// frame itself saying it does not use that password -- rejecting it (401),
// or answering with its authentication in the other state -- as opposed to
// no answer at all, or a lockout (429) that says nothing about the password.
func isDefinitiveNo(err error) bool {
	var se *photoframe.StatusError
	if errors.As(err, &se) {
		return se.StatusCode == 401
	}
	return errors.Is(err, errAuthStateMismatch)
}

var errAuthStateMismatch = errors.New("the frame's authentication is in the other state")

// httpAuthEnabled reads http_auth_enabled from a frame's GET /api/config
// body. Firmware from before frame passwords does not report it.
func httpAuthEnabled(raw string) (bool, error) {
	var cfg struct {
		HTTPAuthEnabled *bool `json:"http_auth_enabled"`
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return false, fmt.Errorf("failed to decode config: %w", err)
	}
	if cfg.HTTPAuthEnabled == nil {
		return false, errors.New("the frame's firmware does not support a password")
	}
	return *cfg.HTTPAuthEnabled, nil
}

// frameCheckTimeout bounds the one request checkFramePassword makes. It is
// made under the device's exclusive lock, which every request to the frame
// waits on, so a frame that takes the connection and then says nothing must
// not hold them for the shared client's two minutes: a config pull has a
// 20-second window. The frame answers /api/config at once.
var frameCheckTimeout = 10 * time.Second

// checkFramePassword asks the frame whether it takes password: a GET
// /api/config with it, or with no credential at all for "". nil means the
// frame confirmed it -- it answered, and reports a password in use; for ""
// an answer to a request with no credential is confirmation enough,
// whatever the firmware. Otherwise the error says why it could not: the
// frame rejected it (FrameWrongPassword), could not be reached
// (FrameUnreachable), is refusing checks for now (FrameLockedOut), does not
// use a password now (FrameOpen), or runs firmware from before passwords
// (FrameUnsupported).
func checkFramePassword(host, password string) *FramePasswordError {
	raw, err := photoframe.NewClientWithPassword(host, password).
		WithTimeout(frameCheckTimeout).FetchConfig()
	if err != nil {
		return framePasswordError(err)
	}
	if password == "" {
		return nil
	}
	enabled, err := httpAuthEnabled(raw)
	if err != nil {
		return &FramePasswordError{Kind: FrameUnsupported, Err: err}
	}
	if !enabled {
		return &FramePasswordError{Kind: FrameOpen}
	}
	return nil
}

// frameUsesPassword reports whether the frame accepts password (no
// Authorization at all for "") and reports its authentication in the matching
// state. The state check matters for an open frame, which accepts any
// password; and when the password is "", no credential is sent, so asking
// never counts against the frame's wrong-guess limit.
func frameUsesPassword(host, password string) (bool, error) {
	raw, err := photoframe.NewClientWithPassword(host, password).FetchConfig()
	if err != nil {
		return false, err
	}
	enabled, err := httpAuthEnabled(raw)
	if err != nil {
		return false, err
	}
	if enabled != (password != "") {
		return false, fmt.Errorf("%w: http_auth_enabled=%v", errAuthStateMismatch, enabled)
	}
	return true, nil
}
