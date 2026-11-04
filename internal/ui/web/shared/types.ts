// What shared overlays keep in the store.

// Confirmation is a question for the user; accept runs if they confirm with
// the primary button.
export interface Confirmation {
  message: string;
  detail: string;
  primary: string;
  accept: () => void;
}

// Rect is an overlay's bounds in CSS pixels.
export interface Rect {
  left: number;
  top: number;
  right: number;
  bottom: number;
}
