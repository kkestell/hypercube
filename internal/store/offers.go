package store

// OpenOffer is a saved attention item that can be answered while its agent is
// parked. Ended sessions keep their offers in the transcript without showing
// them among the user's active requests.
type OpenOffer struct {
	Session string
	Data    []byte
}

func (t *Tx) OpenOffers() ([]OpenOffer, error) {
	// CROSS JOIN makes SQLite read active sessions first and look up only
	// their transcripts, instead of evaluating JSON for every saved item.
	rows, err := t.tx.Query(`SELECT transcript.session, data FROM sessions
  CROSS JOIN transcript ON transcript.session = sessions.id
  WHERE sessions.ended = 0 AND json_extract(data, '$.offer') = 1
  AND COALESCE(json_extract(data, '$.done'), 0) = 0`)
	if err != nil {
		return nil, err
	}
	return collect(rows, func(r interface{ Scan(...any) error }) (OpenOffer, error) {
		var offer OpenOffer
		var data string
		err := r.Scan(&offer.Session, &data)
		offer.Data = []byte(data)
		return offer, err
	})
}
