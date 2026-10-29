package seal

import (
	"bytes"
	"errors"
	"testing"
)

func key(fill byte) []byte { return bytes.Repeat([]byte{fill}, KeySize) }

func TestOpenGivesBackWhatWasSealed(t *testing.T) {
	for _, plaintext := range [][]byte{[]byte("1//0g-refresh-token"), []byte(""), bytes.Repeat([]byte("x"), 4096)} {
		sealed, err := Seal(key(1), plaintext)
		if err != nil {
			t.Fatal(err)
		}
		if len(plaintext) > 0 && bytes.Contains(sealed, plaintext) {
			t.Errorf("the sealed value contains the plaintext")
		}
		opened, err := Open(key(1), sealed)
		if err != nil || !bytes.Equal(opened, plaintext) {
			t.Errorf("Open(Seal(%q)) = %q, %v", plaintext, opened, err)
		}
	}
}

func TestSealingTheSamePlaintextTwiceGivesDifferentValues(t *testing.T) {
	first, _ := Seal(key(1), []byte("token"))
	second, _ := Seal(key(1), []byte("token"))
	if bytes.Equal(first, second) {
		t.Error("two seals of one plaintext are equal, so the nonce is not fresh")
	}
}

func TestAChangedValueDoesNotOpen(t *testing.T) {
	sealed, err := Seal(key(1), []byte("token"))
	if err != nil {
		t.Fatal(err)
	}
	// Every byte: the nonce, the ciphertext and the tag all count.
	for index := range sealed {
		changed := bytes.Clone(sealed)
		changed[index] ^= 0x01
		if opened, err := Open(key(1), changed); !errors.Is(err, ErrOpen) || opened != nil {
			t.Fatalf("byte %d changed: Open = %q, %v; want ErrOpen", index, opened, err)
		}
	}
}

func TestAnotherKeyDoesNotOpen(t *testing.T) {
	sealed, _ := Seal(key(1), []byte("token"))
	if opened, err := Open(key(2), sealed); !errors.Is(err, ErrOpen) || opened != nil {
		t.Errorf("Open with another key = %q, %v; want ErrOpen", opened, err)
	}
}

func TestATruncatedValueDoesNotOpen(t *testing.T) {
	sealed, _ := Seal(key(1), []byte("token"))
	for _, length := range []int{0, 1, 11, 12, 27, len(sealed) - 1} {
		if _, err := Open(key(1), sealed[:length]); !errors.Is(err, ErrOpen) {
			t.Errorf("%d bytes: error = %v, want ErrOpen", length, err)
		}
	}
	if _, err := Open(key(1), nil); !errors.Is(err, ErrOpen) {
		t.Errorf("nil: error = %v, want ErrOpen", err)
	}
}

func TestAKeyThatIsNot32BytesIsRefused(t *testing.T) {
	for _, size := range []int{0, 16, 31, 33, 64} {
		short := bytes.Repeat([]byte{1}, size)
		if _, err := Seal(short, []byte("token")); err == nil || errors.Is(err, ErrOpen) {
			t.Errorf("Seal with a %d-byte key: error = %v, want a key error", size, err)
		}
		if _, err := Open(short, make([]byte, 64)); err == nil || errors.Is(err, ErrOpen) {
			t.Errorf("Open with a %d-byte key: error = %v, want a key error", size, err)
		}
	}
}
