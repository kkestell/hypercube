// What the composer works with: settings, the choices harnesses offer, and
// drafts.
import type {Image} from '../images.ts';

// Settings is a harness, model, and effort a session runs with. An empty
// effort is the harness's default.
export interface Settings {
  harness: string;
  model: string;
  effort: string;
}

// Harness is an installed plugin: its native harness's name, whether that
// harness is installed, or why the plugin could not say.
export interface Harness {
  id: string;
  name?: string;
  installed?: boolean;
  error?: string;
}

// ModelEntry is a manually configured picker choice, independent of sessions.
export interface ModelEntry {
  id: string;
  harness: string;
  model: string;
  efforts: string[];
  default_effort: string;
}

// Draft is what the user has typed and attached but not yet sent, and the
// number of attached images still being read.
export interface Draft {
  text: string;
  images: Image[];
  reading: number;
  choices?: string[];
}
