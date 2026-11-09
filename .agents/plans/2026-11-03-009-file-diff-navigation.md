# File and diff navigation

## Goal

Add shared find, persisted word wrap, and navigation between visible change groups in file and diff views.

## Decisions

The existing transcript find supplies matching and navigation. Share it with file and diff views while retaining transcript behavior. Whole-file patches normally contain a single Git hunk, so consecutive added and removed lines form each navigable change group. Save the shared wrap preference in browser storage; existing layout visibility is not persisted.

## Implementation plan

- Move transcript find and matching into shared modules and adapt their focus scope and presentation for editor views.
- Add a shared wrap setting, its tab bar button and shortcut; apply it to source and diff lines.
- Add diff change navigation buttons and shortcuts using the changed line groups.
- Update usage documentation and add behavior tests for find, wrap and change navigation.

## Test plan

- Find in files and diffs, navigate results and close; retain transcript find behavior.
- Toggle wrap and restore it on reload; verify it applies to both views.
- Navigate separated diff changes in both directions and with keyboard shortcuts.
