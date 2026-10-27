//! `config.json`: the orchestrator's harness, model, and effort, and the named
//! profiles it can give workers.

use std::collections::HashSet;
use std::error::Error;
use std::path::{Path, PathBuf};

use serde::Deserialize;

type Result<T> = std::result::Result<T, Box<dyn Error>>;

/// The harnesses an agent can run.
pub const HARNESSES: [&str; 2] = ["claude", "pi"];

/// A harness, model, and effort for an agent.
#[derive(Clone, Debug, Default, PartialEq, Deserialize)]
#[serde(default)]
pub struct Spec {
    pub harness: String,
    pub model: String,
    pub effort: String,
}

/// A named spec the orchestrator can give a worker. The description tells the
/// orchestrator when to use it.
#[derive(Clone, Debug, Default, PartialEq, Deserialize)]
#[serde(default)]
pub struct Profile {
    pub name: String,
    pub description: String,
    #[serde(flatten)]
    pub spec: Spec,
}

#[derive(Clone, Debug, Default, PartialEq, Deserialize)]
#[serde(default)]
pub struct Config {
    pub orchestrator: Spec,
    pub profiles: Vec<Profile>,
}

/// The location of `config.json`.
pub fn path() -> PathBuf {
    let base = match std::env::var_os("XDG_CONFIG_HOME").filter(|v| !v.is_empty()) {
        Some(base) => PathBuf::from(base),
        None => PathBuf::from(std::env::var_os("HOME").unwrap_or_default()).join(".config"),
    };
    base.join("hypercube").join("config.json")
}

/// Read `config.json` and check that every value is set.
pub fn load() -> Result<Config> {
    load_from(&path())
}

pub fn load_from(path: &Path) -> Result<Config> {
    let data = std::fs::read(path).map_err(|e| format!("read {}: {e}", path.display()))?;
    let config: Config =
        serde_json::from_slice(&data).map_err(|e| format!("{}: {e}", path.display()))?;
    config
        .check()
        .map_err(|e| format!("{}: {e}", path.display()))?;
    Ok(config)
}

impl Config {
    fn check(&self) -> Result<()> {
        self.orchestrator
            .check()
            .map_err(|e| format!("orchestrator: {e}"))?;
        if self.profiles.is_empty() {
            return Err("missing profiles".into());
        }
        let mut seen = HashSet::new();
        for (i, p) in self.profiles.iter().enumerate() {
            if p.name.is_empty() {
                return Err(format!("profile {}: missing name", i + 1).into());
            }
            if !seen.insert(p.name.as_str()) {
                return Err(format!("duplicate profile {:?}", p.name).into());
            }
            if p.description.is_empty() {
                return Err(format!("profile {:?}: missing description", p.name).into());
            }
            p.spec
                .check()
                .map_err(|e| format!("profile {:?}: {e}", p.name))?;
        }
        Ok(())
    }

    /// The profile with the given name.
    pub fn profile(&self, name: &str) -> Result<&Profile> {
        self.profiles
            .iter()
            .find(|p| p.name == name)
            .ok_or_else(|| format!("no profile named {name:?}").into())
    }
}

impl Spec {
    fn check(&self) -> Result<()> {
        if !HARNESSES.contains(&self.harness.as_str()) {
            return Err(format!("harness must be claude or pi, not {:?}", self.harness).into());
        }
        if self.model.is_empty() {
            return Err("missing model".into());
        }
        if self.effort.is_empty() {
            return Err("missing effort".into());
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::testutil::TempDir;

    const ORCHESTRATOR: &str =
        r#""orchestrator": {"harness": "claude", "model": "opus", "effort": "high"}"#;
    const SOL: &str = r#"{"name": "sol", "description": "coding.", "harness": "pi", "model": "openai/gpt-6.1-sol", "effort": "high"}"#;

    fn load_json(json: &str) -> Result<Config> {
        let dir = TempDir::new();
        let path = dir.path().join("config.json");
        std::fs::write(&path, json).unwrap();
        load_from(&path)
    }

    fn error_of(json: &str) -> String {
        load_json(json).unwrap_err().to_string()
    }

    fn with_profiles(profiles: &str) -> String {
        format!(r#"{{{ORCHESTRATOR}, "profiles": {profiles}}}"#)
    }

    #[test]
    fn loads_profiles_for_both_harnesses() {
        let config = load_json(&with_profiles(&format!(
            r#"[{SOL}, {{"name": "fast", "description": "quick.", "harness": "claude", "model": "haiku", "effort": "low"}}]"#
        )))
        .unwrap();
        assert_eq!(config.orchestrator.harness, "claude");
        assert_eq!(config.profiles.len(), 2);
        let sol = config.profile("sol").unwrap();
        assert_eq!(sol.spec.harness, "pi");
        assert_eq!(sol.spec.model, "openai/gpt-6.1-sol");
        assert_eq!(sol.description, "coding.");
        assert_eq!(config.profile("fast").unwrap().spec.model, "haiku");
        assert_eq!(
            config.profile("nope").unwrap_err().to_string(),
            r#"no profile named "nope""#
        );
    }

    #[test]
    fn a_pi_orchestrator_loads() {
        let config = load_json(&format!(
            r#"{{"orchestrator": {{"harness": "pi", "model": "m", "effort": "high"}}, "profiles": [{SOL}]}}"#
        ))
        .unwrap();
        assert_eq!(config.orchestrator.harness, "pi");
    }

    #[test]
    fn unknown_fields_are_ignored() {
        load_json(&format!(
            r#"{{{ORCHESTRATOR}, "extra": 1, "profiles": [{SOL}]}}"#
        ))
        .unwrap();
    }

    #[test]
    fn missing_file_names_the_path() {
        let dir = TempDir::new();
        let err = load_from(&dir.path().join("config.json"))
            .unwrap_err()
            .to_string();
        assert!(
            err.starts_with("read ") && err.contains("config.json"),
            "{err}"
        );
    }

    #[test]
    fn invalid_json_names_the_path() {
        let err = error_of("{");
        assert!(err.contains("config.json"), "{err}");
    }

    #[test]
    fn rejects_an_empty_profile_list() {
        assert!(error_of(&with_profiles("[]")).ends_with(": missing profiles"));
        assert!(error_of(&format!("{{{ORCHESTRATOR}}}")).ends_with(": missing profiles"));
    }

    #[test]
    fn rejects_duplicate_profiles() {
        let err = error_of(&with_profiles(&format!("[{SOL}, {SOL}]")));
        assert!(err.ends_with(r#": duplicate profile "sol""#), "{err}");
    }

    #[test]
    fn rejects_a_profile_without_a_name() {
        let err = error_of(&with_profiles(
            r#"[{"description": "d", "harness": "pi", "model": "m", "effort": "high"}]"#,
        ));
        assert!(err.ends_with(": profile 1: missing name"), "{err}");
    }

    #[test]
    fn rejects_a_profile_without_a_description() {
        let err = error_of(&with_profiles(
            r#"[{"name": "sol", "harness": "pi", "model": "m", "effort": "high"}]"#,
        ));
        assert!(
            err.ends_with(r#": profile "sol": missing description"#),
            "{err}"
        );
    }

    #[test]
    fn rejects_unsupported_harnesses() {
        for harness in ["codex", "opencode", "ox", "gpt", ""] {
            let profile = format!(
                r#"[{{"name": "sol", "description": "d", "harness": "{harness}", "model": "m", "effort": "high"}}]"#
            );
            let err = error_of(&with_profiles(&profile));
            assert!(
                err.ends_with(&format!(
                    r#": profile "sol": harness must be claude or pi, not "{harness}""#
                )),
                "{err}"
            );

            let orchestrator = format!(
                r#"{{"orchestrator": {{"harness": "{harness}", "model": "m", "effort": "high"}}, "profiles": [{SOL}]}}"#
            );
            let err = error_of(&orchestrator);
            assert!(
                err.ends_with(&format!(
                    r#": orchestrator: harness must be claude or pi, not "{harness}""#
                )),
                "{err}"
            );
        }
    }

    #[test]
    fn rejects_a_missing_model_or_effort() {
        for (field, json) in [
            ("model", r#"{"harness": "pi", "effort": "high"}"#),
            ("effort", r#"{"harness": "pi", "model": "m"}"#),
        ] {
            let err = error_of(&format!(
                r#"{{"orchestrator": {json}, "profiles": [{SOL}]}}"#
            ));
            assert!(
                err.ends_with(&format!(": orchestrator: missing {field}")),
                "{err}"
            );
        }
        let err = error_of(&with_profiles(
            r#"[{"name": "sol", "description": "d", "harness": "pi", "effort": "high"}]"#,
        ));
        assert!(err.ends_with(r#": profile "sol": missing model"#), "{err}");
        let err = error_of(&with_profiles(
            r#"[{"name": "sol", "description": "d", "harness": "pi", "model": "m"}]"#,
        ));
        assert!(err.ends_with(r#": profile "sol": missing effort"#), "{err}");
    }

    #[test]
    fn rejects_a_missing_orchestrator() {
        let err = error_of(&format!(r#"{{"profiles": [{SOL}]}}"#));
        assert!(
            err.contains("orchestrator: harness must be claude or pi"),
            "{err}"
        );
    }
}
