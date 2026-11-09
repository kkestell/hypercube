# Follow system appearance

## Goal

Follow the system's light or dark appearance across the interface, updating live.

## Decisions

Keep shadcn's stock class-based theme and synchronize its class with the system. Use Shiki's dual-theme tokens with GitHub Light and After Dark. Let AppKit choose the native window appearance.

## Implementation plan

- Synchronize the page theme with the system and remove the hardcoded class.
- Apply matching terminal colors, syntax colors, and tesseract lines as appearance changes. Clean up listeners with their owners.
- Verify both syntax palettes and live theme behavior, and document the system appearance in the user guide.
- Regenerate the browser bundle and run the required checks.
