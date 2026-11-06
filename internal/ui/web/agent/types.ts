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

// Model is one model a harness offers: its native ID, and its label,
// provider, and the model an alias stands for when the harness reports them.
export interface Model {
  id: string;
  label?: string;
  provider?: string;
  resolved?: string;
}

// Catalog is what a harness offers in a directory: its models and default
// model and, for a requested model, its efforts in the harness's order and
// its default effort.
export interface Catalog {
  models: Model[] | null;
  default?: string;
  model?: string;
  efforts?: string[];
  default_effort?: string;
}

// Draft is what the user has typed and attached but not yet sent, and the
// number of attached images still being read.
export interface Draft {
  text: string;
  images: Image[];
  reading: number;
  choices?: string[];
}
