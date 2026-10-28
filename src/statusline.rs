//! `hypercube statusline`: the status line Claude Code shows under each agent's
//! prompt. It shows the model, the effort, and the context window in use, and
//! for the orchestrator the 5-hour and 7-day usage limits. It also clears the
//! agent's attention flag, since Claude Code runs the status line when a tool
//! approval is answered, either way, and not while it waits for an answer.

use std::error::Error;
use std::io::{Read, Write};
use std::path::Path;

use serde_json::Value;

use crate::store;

type Result<T> = std::result::Result<T, Box<dyn Error>>;

const ORANGE: &str = "38;2;218;96;0";
const WHITE: &str = "38;2;255;255;255";
const LIGHT_ORANGE: &str = "38;2;255;151;69";

/// Clear the attention flag of the agent named by `HYPERCUBE_AGENT_ID` and
/// `HYPERCUBE_STATE`, then show the status line.
pub fn run(limits: bool, input: impl Read, out: impl Write) -> Result<()> {
    let id = std::env::var("HYPERCUBE_AGENT_ID").unwrap_or_default();
    let state = std::env::var("HYPERCUBE_STATE").unwrap_or_default();
    if !id.is_empty() && !state.is_empty() {
        // The status line shows even if the state cannot be changed.
        let _ = clear_attention(&id, Path::new(&state));
    }
    show(limits, input, out)
}

/// Read Claude Code's status JSON from `input` and write the status line.
/// `limits` adds the usage limits.
fn show(limits: bool, mut input: impl Read, mut out: impl Write) -> Result<()> {
    let mut text = String::new();
    input.read_to_string(&mut text)?;
    let status: Value = serde_json::from_str(&text).unwrap_or(Value::Null);
    out.write_all(line(&status, limits).as_bytes())?;
    Ok(())
}

/// Clear agent `id`'s attention flag. The state is only written when the
/// flag is set, since the status line runs often.
fn clear_attention(id: &str, state_path: &Path) -> Result<()> {
    if !store::load(state_path)?.agent(id)?.attention {
        return Ok(());
    }
    store::update(state_path, |state| {
        state.agent_mut(id)?.attention = false;
        Ok(())
    })
}

/// The status line for Claude Code's status JSON. Missing or null fields
/// count as empty or zero.
fn line(status: &Value, limits: bool) -> String {
    let text = |path| status.pointer(path).and_then(Value::as_str).unwrap_or("");
    let number = |path| status.pointer(path).and_then(Value::as_f64).unwrap_or(0.0);

    let mut s = paint(ORANGE, "✳︎ ") + &paint(ORANGE, text("/model/display_name"));
    s += &paint(WHITE, " :: ");
    if status.pointer("/thinking/enabled") == Some(&Value::Bool(true)) {
        s += &paint(LIGHT_ORANGE, text("/effort/level"));
    }
    s += &format!(
        " {}/{} ({}%)",
        tokens(number("/context_window/total_input_tokens")),
        tokens(number("/context_window/context_window_size")),
        number("/context_window/used_percentage") as u64,
    );
    if limits {
        s += &format!(
            " | 5h {} | 7d {}",
            bar(number("/rate_limits/five_hour/used_percentage")),
            bar(number("/rate_limits/seven_day/used_percentage")),
        );
    }
    s
}

fn paint(sgr: &str, text: &str) -> String {
    format!("\x1b[{sgr}m{text}\x1b[0m")
}

/// A token count in thousands or millions with at most one decimal: 950,
/// 12k, 12.3k, 1M, 1.2M.
fn tokens(n: f64) -> String {
    let n = n as u64;
    let (unit, scale) = match n {
        0..1_000 => return n.to_string(),
        1_000..1_000_000 => ("k", 1_000),
        _ => ("M", 1_000_000),
    };
    let (whole, tenth) = (n / scale, n % scale / (scale / 10));
    if tenth == 0 {
        format!("{whole}{unit}")
    } else {
        format!("{whole}.{tenth}{unit}")
    }
}

/// A ten-cell bar for a percentage.
fn bar(percent: f64) -> String {
    let filled = ((percent.clamp(0.0, 100.0) as usize) * 10 + 50) / 100;
    "▪".repeat(filled) + &"·".repeat(10 - filled)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn plain(s: &str) -> String {
        let mut out = String::new();
        let mut chars = s.chars();
        while let Some(c) = chars.next() {
            if c == '\x1b' {
                chars.by_ref().find(|&c| c == 'm');
            } else {
                out.push(c);
            }
        }
        out
    }

    fn status() -> Value {
        json!({
            "model": {"display_name": "Opus"},
            "thinking": {"enabled": true},
            "effort": {"level": "medium"},
            "context_window": {
                "total_input_tokens": 45_300,
                "context_window_size": 200_000,
                "used_percentage": 22.6,
            },
            "rate_limits": {
                "five_hour": {"used_percentage": 34},
                "seven_day": {"used_percentage": 71.5},
            },
        })
    }

    #[test]
    fn the_orchestrator_line_has_the_limits() {
        assert_eq!(
            plain(&line(&status(), true)),
            "✳︎ Opus :: medium 45.3k/200k (22%) | 5h ▪▪▪······· | 7d ▪▪▪▪▪▪▪···"
        );
    }

    #[test]
    fn a_worker_line_has_no_limits() {
        assert_eq!(
            plain(&line(&status(), false)),
            "✳︎ Opus :: medium 45.3k/200k (22%)"
        );
    }

    #[test]
    fn colors_mark_the_model_and_effort() {
        let s = line(&status(), false);
        assert!(s.starts_with("\x1b[38;2;218;96;0m✳︎ \x1b[0m\x1b[38;2;218;96;0mOpus\x1b[0m"));
        assert!(s.contains("\x1b[38;2;255;151;69mmedium\x1b[0m"));
    }

    #[test]
    fn the_effort_shows_only_with_thinking() {
        let mut s = status();
        s["thinking"]["enabled"] = json!(false);
        assert_eq!(plain(&line(&s, false)), "✳︎ Opus ::  45.3k/200k (22%)");
    }

    #[test]
    fn missing_and_null_fields_count_as_empty() {
        let s = json!({"context_window": {"used_percentage": null}, "rate_limits": null});
        assert_eq!(
            plain(&line(&s, true)),
            "✳︎  ::  0/0 (0%) | 5h ·········· | 7d ··········"
        );
        let mut out = Vec::new();
        show(true, "not json".as_bytes(), &mut out).unwrap();
        assert!(String::from_utf8(out).unwrap().contains("0/0 (0%)"));
    }

    #[test]
    fn the_status_line_clears_the_attention_flag() {
        let dir = crate::testutil::TempDir::new();
        let state = dir.path().join("w.json");
        let agent = |id: &str, attention| store::Agent {
            id: id.into(),
            name: id.into(),
            attention,
            ..store::Agent::default()
        };
        store::update(&state, |s| {
            s.agents = vec![agent("a", true), agent("b", true)];
            Ok(())
        })
        .unwrap();
        clear_attention("a", &state).unwrap();
        let attention: Vec<_> = store::load(&state)
            .unwrap()
            .agents
            .iter()
            .map(|a| a.attention)
            .collect();
        assert_eq!(attention, [false, true]);
        // A clear agent's state is left unwritten.
        let written = std::fs::metadata(&state).unwrap().modified().unwrap();
        clear_attention("a", &state).unwrap();
        assert_eq!(
            std::fs::metadata(&state).unwrap().modified().unwrap(),
            written
        );
        assert!(clear_attention("nobody", &state).is_err());
    }

    #[test]
    fn token_counts_are_compact() {
        for (n, want) in [
            (0.0, "0"),
            (950.0, "950"),
            (12_000.0, "12k"),
            (12_345.0, "12.3k"),
            (1_000_000.0, "1M"),
            (1_250_000.0, "1.2M"),
        ] {
            assert_eq!(tokens(n), want);
        }
    }

    #[test]
    fn bars_round_to_the_nearest_cell_and_clamp() {
        assert_eq!(bar(0.0), "··········");
        assert_eq!(bar(5.0), "▪·········");
        assert_eq!(bar(44.0), "▪▪▪▪······");
        assert_eq!(bar(150.0), "▪▪▪▪▪▪▪▪▪▪");
    }
}
