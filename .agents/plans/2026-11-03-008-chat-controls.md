# Chat controls

## Goal

Support pasted images, return to the newest transcript output, and interrupt a working agent from an empty composer.

## Implementation plan

- Route clipboard image files from `agent/composer.jsx` through `attachImages`; leave text paste to the browser.
- Show a stop action for a running busy session with an empty composer, using the same interrupt request as Escape. Add the square icon to `shared/icon.jsx`.
- Use the transcript's existing follow state to show a centered button that returns to the end in `transcript/transcript.jsx`.
- Update `docs/usage.md` and exercise paste, stop, and scrolling in the existing component tests.

## Test plan

- Check image paste, ordinary text paste, attachment limits, stop/send switching and interruption, and returning to newest output.
