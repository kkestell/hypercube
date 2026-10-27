//! Hypercube runs a team of local coding agents, each a Claude Code or Pi
//! session in its own pane.

use std::process::ExitCode;

const USAGE: &str = "usage: hypercube [stop [dir] | delete [dir] | status | mcp | hook <event>]";

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let args: Vec<&str> = args.iter().map(String::as_str).collect();
    let command = match args.as_slice() {
        [] => "start",
        ["stop" | "delete"] | ["stop" | "delete", _] | ["status"] | ["mcp"] | ["hook", _] => {
            args[0]
        }
        _ => {
            eprintln!("{USAGE}");
            return ExitCode::from(2);
        }
    };
    eprintln!("hypercube: {command} is not implemented yet");
    ExitCode::FAILURE
}
