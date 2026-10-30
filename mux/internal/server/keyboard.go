package server

import (
	"fmt"
	"strconv"
	"strings"

	"go.mitchellh.com/libghostty"
)

const prefixKey = 0x02 // Ctrl-b

const (
	modCtrl  = 4
	modLocks = 64 | 128 // Caps Lock and Num Lock, which never change a command

	kittyModifierFirst, kittyModifierLast = 57441, 57454 // Shift, Ctrl, and the other modifier keys
	pressEvent                            = 1            // kitty event types are 1 press, 2 repeat, 3 release
)

// key is one key as the outer terminal sent it, in any keyboard mode.
type key struct {
	code  int // the key's character, or 0 for a key filter does not decode
	mods  int // the modifier bitmask, without the lock bits
	event int
}

func (k key) isPrefix() bool {
	return k.code == 'b' && k.mods == modCtrl && k.event == pressEvent
}

func (k key) isModifier() bool {
	return k.code >= kittyModifierFirst && k.code <= kittyModifierLast
}

// parseKey decodes the key at the start of data, which is not empty, and returns
// how many bytes it took. It recognizes a single byte, a kitty key
// (CSI code ; mods:event u), and a modifyOtherKeys key (CSI 27 ; mods ; code ~).
// Any other escape sequence is one key with code 0. A sequence cut off by the
// end of data is taken as it is; that is unlikely.
func parseKey(data []byte) (key, int) {
	if data[0] != 0x1b || len(data) == 1 {
		if data[0] == prefixKey {
			return key{code: 'b', mods: modCtrl, event: pressEvent}, 1
		}
		// A multibyte character is several keys, so one typed after the prefix
		// passes its trailing bytes to the pane; that is unlikely.
		return key{code: int(data[0]), event: pressEvent}, 1
	}
	if data[1] != '[' {
		if data[1] == 'O' && len(data) > 2 {
			return key{event: pressEvent}, 3
		}
		return key{event: pressEvent}, 2
	}
	end := 2
	for end < len(data) && (data[end] < 0x40 || data[end] > 0x7e) {
		end++
	}
	if end == len(data) {
		return key{event: pressEvent}, end
	}
	n := end + 1
	fields := strings.Split(string(data[2:end]), ";")
	switch {
	case data[end] == 'u':
		code, _, _ := strings.Cut(fields[0], ":")
		var mods, event string
		if len(fields) > 1 {
			mods, event, _ = strings.Cut(fields[1], ":")
		}
		return newKey(code, mods, event), n
	case data[end] == '~' && len(fields) == 3 && fields[0] == "27":
		return newKey(fields[2], fields[1], ""), n
	}
	return key{event: pressEvent}, n
}

// newKey builds a key from the text of a kitty or modifyOtherKeys sequence. An
// empty modifier field means none and an empty event means a press.
func newKey(code, mods, event string) key {
	k := key{event: pressEvent}
	k.code, _ = strconv.Atoi(code)
	if m, err := strconv.Atoi(mods); err == nil && m > 0 {
		k.mods = (m - 1) &^ modLocks
	}
	if e, err := strconv.Atoi(event); err == nil {
		k.event = e
	}
	return k
}

// filter takes the prefix key and the key after it out of input, which the
// client sent in the focused pane's keyboard mode. It returns the bytes for the
// pane, then the command the key after the prefix names or the mouse report it
// stopped at, and whatever input follows. With neither, cmd is 0, mouse is nil,
// and rest is empty. A mouse report leaves a pending prefix pending.
//
// A key sequence split across two reads passes through undecoded, and a paste
// containing the prefix byte starts the prefix. With kitty event types, the
// release of a command key goes to the pane focused after the command, and with
// all keys as escape codes the Ctrl press before the prefix reaches the pane.
// Those are unlikely.
func (a *attachment) filter(data []byte) (pass []byte, cmd rune, mouse *mouseEvent, rest []byte) {
	for len(data) > 0 {
		k, n := parseKey(data)
		raw := data[:n]
		data = data[n:]
		if ev, ok := parseMouse(raw); ok {
			return pass, 0, &ev, data
		}
		switch {
		case !a.prefix:
			if k.isPrefix() {
				a.prefix = true
			} else {
				pass = append(pass, raw...)
			}
		// Panes that ask for event types or all keys as escape codes send these
		// between the prefix and the command.
		case k.event != pressEvent || k.isModifier():
		case k.isPrefix():
			a.prefix = false
			pass = append(pass, raw...)
		default:
			a.prefix = false
			if k.mods == 0 && k.code != 0 {
				return pass, rune(k.code), nil, data
			}
		}
	}
	return pass, 0, nil, nil
}

// appendKeyboardModes appends the sequences that put the outer terminal in the
// pane's keyboard modes: cursor keys, keypad, bracketed paste, the kitty
// keyboard flags, and modifyOtherKeys. libghostty has no getter for
// modifyOtherKeys, so a key encoder tells whether the pane set it: a Ctrl-b
// press encodes as a modifyOtherKeys sequence only if it did. Focus reporting
// and mouse modes are not mirrored. It must be called with the mutex held.
func appendKeyboardModes(b []byte, p *pane) ([]byte, error) {
	cursorKeys, err := p.term.Mode(libghostty.ModeDECCKM)
	if err != nil {
		return nil, err
	}
	keypad, err := p.term.Mode(libghostty.ModeKeypadKeys)
	if err != nil {
		return nil, err
	}
	paste, err := p.term.Mode(libghostty.ModeBracketedPaste)
	if err != nil {
		return nil, err
	}
	flags, err := p.term.KittyKeyboardFlags()
	if err != nil {
		return nil, err
	}
	otherKeys, err := modifyOtherKeys(p.term)
	if err != nil {
		return nil, err
	}

	b = append(b, choose(cursorKeys, "\x1b[?1h", "\x1b[?1l")...)
	b = append(b, choose(keypad, "\x1b=", "\x1b>")...)
	b = append(b, choose(paste, "\x1b[?2004h", "\x1b[?2004l")...)
	b = fmt.Appendf(b, "\x1b[=%d;1u", flags)
	return append(b, choose(otherKeys, "\x1b[>4;2m", "\x1b[>4;0m")...), nil
}

func choose(on bool, yes, no string) string {
	if on {
		return yes
	}
	return no
}

// modifyOtherKeys reports whether the terminal is in modifyOtherKeys level 2.
// libghostty tracks no other level, and level 1 does not change how Ctrl-b is
// encoded.
func modifyOtherKeys(t *libghostty.Terminal) (bool, error) {
	enc, err := libghostty.NewKeyEncoder()
	if err != nil {
		return false, err
	}
	defer enc.Close()
	enc.SetOptFromTerminal(t)
	enc.SetOptKittyFlags(libghostty.KittyKeyDisabled)
	ev, err := libghostty.NewKeyEvent()
	if err != nil {
		return false, err
	}
	defer ev.Close()
	ev.SetAction(libghostty.KeyActionPress)
	ev.SetKey(libghostty.KeyB)
	ev.SetMods(libghostty.ModCtrl)
	ev.SetUTF8("b")
	ev.SetUnshiftedCodepoint('b')
	out, err := enc.Encode(ev)
	if err != nil {
		return false, err
	}
	return string(out) != "\x02", nil
}
