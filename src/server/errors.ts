import type { UnsavedWork } from "../model";

/** A request the user can fix: its message goes back to the browser, and it isn't logged as a failure. */
export class ModelError extends Error {}

/** A delete refused because the session holds work that exists nowhere else. The user may confirm and delete anyway. */
export class UnsavedWorkError extends ModelError {
  constructor(readonly unsaved: UnsavedWork[]) {
    super("This session has work that isn't on GitHub");
  }
}
