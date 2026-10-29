package store_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/calendarconnection"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/privacy"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/viewer"
)

func TestCalendarConnectionKeepsItsFields(t *testing.T) {
	client, asAlice, _ := twoUsers(t)
	aliceID, _ := viewer.UserID(asAlice)
	sealed := []byte{0x01, 0x02, 0x03, 0xff}
	created := client.CalendarConnection.Create().SetUserID(aliceID).SetSubject("1157").SetEmail("alice@gmail.com").
		SetSealedToken(sealed).SaveX(asAlice)

	read := client.CalendarConnection.GetX(asAlice, created.ID)
	if read.Subject != "1157" || read.Email != "alice@gmail.com" || !bytes.Equal(read.SealedToken, sealed) || read.CreatedAt.IsZero() {
		t.Errorf("read back %+v", read)
	}
	if read.Status != calendarconnection.StatusConnected {
		t.Errorf("a new connection is %q, want connected", read.Status)
	}
	// Reconnecting changes the account, the token and the status in place.
	client.CalendarConnection.UpdateOne(read).SetStatus(calendarconnection.StatusNeedsReconnect).SaveX(asAlice)
	if again := client.CalendarConnection.GetX(asAlice, created.ID); again.Status != calendarconnection.StatusNeedsReconnect {
		t.Errorf("status = %q, want needs_reconnect", again.Status)
	}
	client.CalendarConnection.UpdateOne(read).SetStatus(calendarconnection.StatusConnected).
		SetSubject("2290").SetEmail("alice@work.example").SetSealedToken([]byte{0x09}).SaveX(asAlice)
	if again := client.CalendarConnection.GetX(asAlice, created.ID); again.Subject != "2290" || again.Email != "alice@work.example" ||
		!bytes.Equal(again.SealedToken, []byte{0x09}) || again.Status != calendarconnection.StatusConnected {
		t.Errorf("after reconnecting: %+v", again)
	}
	// The token is left out when a row is printed, sealed or not.
	if text := read.String(); !strings.Contains(text, "sealed_token=<sensitive>") {
		t.Errorf("String() = %s, want the sealed token redacted", text)
	}
}

func TestAUserHasOneCalendarConnection(t *testing.T) {
	client, asAlice, asBob := twoUsers(t)
	aliceID, _ := viewer.UserID(asAlice)
	bobID, _ := viewer.UserID(asBob)

	client.CalendarConnection.Create().SetUserID(aliceID).SetSubject("1").SetEmail("alice@gmail.com").SetSealedToken([]byte{1}).SaveX(asAlice)
	_, err := client.CalendarConnection.Create().SetUserID(aliceID).SetSubject("2").SetEmail("other@gmail.com").SetSealedToken([]byte{2}).Save(asAlice)
	if !ent.IsConstraintError(err) {
		t.Errorf("a second connection for Alice: %v, want a constraint error", err)
	}
	if _, err := client.CalendarConnection.Create().SetUserID(bobID).SetSubject("3").SetEmail("bob@gmail.com").SetSealedToken([]byte{3}).Save(asBob); err != nil {
		t.Errorf("Bob's connection: %v", err)
	}
}

func TestUsersCannotReachEachOthersCalendarRows(t *testing.T) {
	client, asAlice, asBob := twoUsers(t)
	aliceID, _ := viewer.UserID(asAlice)
	bobID, _ := viewer.UserID(asBob)
	connection := client.CalendarConnection.Create().SetUserID(aliceID).SetSubject("1").SetEmail("alice@gmail.com").SetSealedToken([]byte{1}).SaveX(asAlice)
	link := client.CalendarLink.Create().SetUserID(aliceID).SetLinkHash([]byte{1, 1}).SetExpiresAt(time.Now().Add(10 * time.Minute)).SaveX(asAlice)

	if rows := client.CalendarConnection.Query().AllX(asBob); len(rows) != 0 {
		t.Errorf("Bob lists %d of Alice's connections", len(rows))
	}
	if rows := client.CalendarLink.Query().AllX(asBob); len(rows) != 0 {
		t.Errorf("Bob lists %d of Alice's links", len(rows))
	}
	if _, err := client.CalendarConnection.Get(asBob, connection.ID); !ent.IsNotFound(err) {
		t.Errorf("Bob gets Alice's connection: %v", err)
	}
	if _, err := client.CalendarLink.Get(asBob, link.ID); !ent.IsNotFound(err) {
		t.Errorf("Bob gets Alice's link: %v", err)
	}
	if _, err := client.CalendarConnection.UpdateOneID(connection.ID).SetEmail("bob@gmail.com").Save(asBob); !ent.IsNotFound(err) {
		t.Errorf("Bob updates Alice's connection: %v", err)
	}
	if _, err := client.CalendarLink.UpdateOneID(link.ID).SetNonce("planted").Save(asBob); !ent.IsNotFound(err) {
		t.Errorf("Bob updates Alice's link: %v", err)
	}
	if deleted := client.CalendarConnection.Delete().ExecX(asBob); deleted != 0 {
		t.Errorf("Bob's bulk delete removes %d of Alice's connections", deleted)
	}
	if deleted := client.CalendarLink.Delete().ExecX(asBob); deleted != 0 {
		t.Errorf("Bob's bulk delete removes %d of Alice's links", deleted)
	}
	if _, err := client.CalendarConnection.Create().SetUserID(aliceID).SetSubject("2").SetEmail("bob@gmail.com").SetSealedToken([]byte{2}).Save(asBob); !errors.Is(err, privacy.Deny) {
		t.Errorf("Bob creates a connection for Alice: %v", err)
	}
	if _, err := client.CalendarLink.Create().SetUserID(aliceID).SetLinkHash([]byte{2, 2}).SetExpiresAt(time.Now().Add(time.Minute)).Save(asBob); !errors.Is(err, privacy.Deny) {
		t.Errorf("Bob creates a link for Alice: %v", err)
	}
	if _, err := client.CalendarLink.Create().SetUserID(bobID).SetLinkHash([]byte{3, 3}).SetExpiresAt(time.Now().Add(time.Minute)).Save(asBob); err != nil {
		t.Errorf("Bob creates his own link: %v", err)
	}

	// Alice's rows are as she left them.
	if read := client.CalendarConnection.GetX(asAlice, connection.ID); read.Email != "alice@gmail.com" {
		t.Errorf("Alice's connection is now %q", read.Email)
	}
	if links := client.CalendarLink.Query().AllX(asAlice); len(links) != 1 {
		t.Errorf("Alice lists %d links, want only hers", len(links))
	}
}

func TestCalendarRowsNeedASignedInUser(t *testing.T) {
	client, asAlice, _ := twoUsers(t)
	aliceID, _ := viewer.UserID(asAlice)
	client.CalendarConnection.Create().SetUserID(aliceID).SetSubject("1").SetEmail("alice@gmail.com").SetSealedToken([]byte{1}).SaveX(asAlice)
	nobody := t.Context()

	if _, err := client.CalendarConnection.Query().All(nobody); !errors.Is(err, privacy.Deny) {
		t.Errorf("connection query without a user: %v", err)
	}
	if _, err := client.CalendarLink.Query().All(nobody); !errors.Is(err, privacy.Deny) {
		t.Errorf("link query without a user: %v", err)
	}
	if _, err := client.CalendarConnection.Delete().Exec(nobody); !errors.Is(err, privacy.Deny) {
		t.Errorf("connection delete without a user: %v", err)
	}
	if _, err := client.CalendarLink.Create().SetUserID(aliceID).SetLinkHash([]byte{4, 4}).SetExpiresAt(time.Now().Add(time.Minute)).Save(nobody); !errors.Is(err, privacy.Deny) {
		t.Errorf("link create without a user: %v", err)
	}
}

func TestCalendarLinksKeepTheirRoundTripAndStayUnique(t *testing.T) {
	client, asAlice, asBob := twoUsers(t)
	aliceID, _ := viewer.UserID(asAlice)
	bobID, _ := viewer.UserID(asBob)
	expires := time.Now().Add(10 * time.Minute)
	link := client.CalendarLink.Create().SetUserID(aliceID).SetLinkHash([]byte{1, 1}).SetExpiresAt(expires).SaveX(asAlice)
	if link.StateHash != nil || link.CodeVerifier != "" || link.Nonce != "" {
		t.Errorf("a new link already has a round trip: %+v", link)
	}
	// Opening the link in the browser starts the Google round trip.
	client.CalendarLink.UpdateOne(link).SetStateHash([]byte{9, 9}).SetCodeVerifier("verifier").SetNonce("nonce").SaveX(asAlice)
	read := client.CalendarLink.GetX(asAlice, link.ID)
	if read.StateHash == nil || !bytes.Equal(*read.StateHash, []byte{9, 9}) || read.CodeVerifier != "verifier" || read.Nonce != "nonce" || !read.ExpiresAt.Equal(expires.Truncate(time.Microsecond)) {
		t.Errorf("read back %+v", read)
	}
	// Two links never share a link hash or a state hash, whoever made them.
	if _, err := client.CalendarLink.Create().SetUserID(bobID).SetLinkHash([]byte{1, 1}).SetExpiresAt(expires).Save(asBob); !ent.IsConstraintError(err) {
		t.Errorf("a repeated link hash: %v, want a constraint error", err)
	}
	other := client.CalendarLink.Create().SetUserID(bobID).SetLinkHash([]byte{2, 2}).SetExpiresAt(expires).SaveX(asBob)
	if _, err := client.CalendarLink.UpdateOne(other).SetStateHash([]byte{9, 9}).Save(asBob); !ent.IsConstraintError(err) {
		t.Errorf("a repeated state hash: %v, want a constraint error", err)
	}
}
