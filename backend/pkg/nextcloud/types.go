package nextcloud

import (
	"encoding/json"
	"strconv"
	"strings"
)

// ClusterID is an album's cluster id as Memories reports it. Memories types
// it as number | string (albums use "owner/name"), so it is decoded from
// either JSON form into its string value.
type ClusterID string

// UnmarshalJSON accepts a JSON string or number.
func (c *ClusterID) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*c = ClusterID(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*c = ClusterID(n.String())
	return nil
}

// Album is one entry of GET /api/clusters/albums: an album the user owns or
// one shared with them.
type Album struct {
	AlbumID   int64     `json:"album_id"`
	ClusterID ClusterID `json:"cluster_id"`
	Name      string    `json:"name"`
	User      string    `json:"user"` // owner
	Count     int       `json:"count"`
}

// Day is one entry of GET /api/days: a day id (days since the Unix epoch, of
// the photo's local date) and how many photos fall on it.
type Day struct {
	DayID int64 `json:"dayid"`
	Count int   `json:"count"`
}

// Photo is one file of GET /api/days/{ids}. Only the fields used are
// declared.
type Photo struct {
	FileID   int64    `json:"fileid"`
	Etag     string   `json:"etag"`
	Basename string   `json:"basename"`
	Mimetype string   `json:"mimetype"`
	DayID    int64    `json:"dayid"`
	W        int      `json:"w"`
	H        int      `json:"h"`
	IsVideo  flexBool `json:"isvideo"`
	IsHidden flexBool `json:"ishidden"`
	// DateTaken is the taken date in epoch seconds when the server includes
	// it; the day listing usually leaves it out (DayID still dates the photo).
	DateTaken int64 `json:"datetaken"`
}

// flexBool decodes a JSON bool, a 0/1 number, or a "0"/"1"/"true" string:
// PHP backends are not always consistent about which they send.
type flexBool bool

// UnmarshalJSON implements json.Unmarshaler.
func (f *flexBool) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	switch strings.ToLower(s) {
	case "", "null", "false", "0":
		*f = false
		return nil
	case "true":
		*f = true
		return nil
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*f = n != 0
	return nil
}

// Filter narrows a day query: one album (its cluster id), favorites only, or
// neither (the whole timeline).
type Filter struct {
	Album     string
	Favorites bool
}
