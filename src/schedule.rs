//! Scheduled tasks' cron schedules, and their firing: a matching task queues
//! its prompt for its orchestrator as a message from `store::SCHEDULE`.

use std::error::Error;
use std::time::{SystemTime, UNIX_EPOCH};

use crate::store::{self, Message, State};

type Result<T> = std::result::Result<T, Box<dyn Error>>;

/// Five cron fields, minute hour day-of-month month day-of-week, in local
/// time. Each field is `*`, a number, `a-b`, `*/s`, `a-b/s`, or a comma list
/// of these. Day-of-week is 0 to 7, with 0 and 7 both Sunday.
#[derive(Debug, PartialEq)]
pub struct Schedule {
    /// Each field's matching values, as bits.
    minutes: u64,
    hours: u64,
    days: u64,
    months: u64,
    weekdays: u64,
    /// Whether each day field is restricted, not starting with `*`. When both
    /// are, a day matching either matches.
    days_restricted: bool,
    weekdays_restricted: bool,
}

/// A minute of local time.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct LocalMinute {
    pub minute: u32,
    pub hour: u32,
    /// From 1.
    pub day: u32,
    /// From 1.
    pub month: u32,
    /// From 0, Sunday.
    pub weekday: u32,
}

impl Schedule {
    pub fn parse(text: &str) -> Result<Schedule> {
        let fields: Vec<&str> = text.split_whitespace().collect();
        let [minutes, hours, days, months, weekdays] = fields[..] else {
            return Err(format!(
                "a schedule has five fields, minute hour day-of-month month day-of-week: {text:?}"
            )
            .into());
        };
        let mut weekday_bits = field(weekdays, 0, 7, "day-of-week")?;
        // 7 is Sunday too.
        if weekday_bits & 1 << 7 != 0 {
            weekday_bits = (weekday_bits | 1) & !(1 << 7);
        }
        Ok(Schedule {
            minutes: field(minutes, 0, 59, "minute")?,
            hours: field(hours, 0, 23, "hour")?,
            days: field(days, 1, 31, "day-of-month")?,
            months: field(months, 1, 12, "month")?,
            weekdays: weekday_bits,
            days_restricted: !days.starts_with('*'),
            weekdays_restricted: !weekdays.starts_with('*'),
        })
    }

    pub fn matches(&self, at: &LocalMinute) -> bool {
        let has = |bits: u64, n: u32| bits & 1 << n != 0;
        let day = has(self.days, at.day);
        let weekday = has(self.weekdays, at.weekday);
        let day = if self.days_restricted && self.weekdays_restricted {
            day || weekday
        } else {
            day && weekday
        };
        has(self.minutes, at.minute)
            && has(self.hours, at.hour)
            && has(self.months, at.month)
            && day
    }
}

/// The values of one cron field from `min` to `max`, as bits.
fn field(text: &str, min: u32, max: u32, name: &str) -> Result<u64> {
    let invalid = || format!("invalid {name} field {text:?}");
    let number = |s: &str| -> Result<u32> {
        let n: u32 = s.parse().map_err(|_| invalid())?;
        if !(min..=max).contains(&n) {
            return Err(format!("{name} {n} is outside {min}-{max}").into());
        }
        Ok(n)
    };
    let mut bits = 0;
    for item in text.split(',') {
        let (range, step) = match item.split_once('/') {
            Some((range, step)) => {
                let step: u32 = step.parse().map_err(|_| invalid())?;
                if step == 0 {
                    return Err(invalid().into());
                }
                (range, Some(step))
            }
            None => (item, None),
        };
        let (from, to) = match range.split_once('-') {
            _ if range == "*" => (min, max),
            Some((a, b)) => (number(a)?, number(b)?),
            // A step needs a range.
            None if step.is_some() => return Err(invalid().into()),
            None => {
                let n = number(range)?;
                (n, n)
            }
        };
        if from > to {
            return Err(invalid().into());
        }
        for n in (from..=to).step_by(step.unwrap_or(1) as usize) {
            bits |= 1 << n;
        }
    }
    Ok(bits)
}

impl LocalMinute {
    /// The local minute of `secs` Unix seconds.
    pub fn at(secs: u64) -> LocalMinute {
        let time = secs as libc::time_t;
        let mut tm: libc::tm = unsafe { std::mem::zeroed() };
        unsafe { libc::localtime_r(&time, &mut tm) };
        LocalMinute {
            minute: tm.tm_min as u32,
            hour: tm.tm_hour as u32,
            day: tm.tm_mday as u32,
            month: tm.tm_mon as u32 + 1,
            weekday: tm.tm_wday as u32,
        }
    }

    pub fn now() -> LocalMinute {
        LocalMinute::at(unix_now())
    }
}

/// The time in Unix seconds.
pub fn unix_now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |d| d.as_secs())
}

/// Fire every task whose schedule matches `at`: queue its prompt for its
/// orchestrator as a message from `store::SCHEDULE`, and set its last run to
/// `now`. A running task, a task whose last firing is still queued, a task
/// without an orchestrator, and a task whose schedule no longer parses are
/// skipped. Returns the names of the tasks fired.
pub fn fire(state: &mut State, at: &LocalMinute, now: u64) -> Vec<String> {
    let mut fired = Vec::new();
    for i in 0..state.tasks.len() {
        let task = &state.tasks[i];
        if !Schedule::parse(&task.schedule).is_ok_and(|s| s.matches(at))
            || state.running(&task.name)
        {
            continue;
        }
        let Some(orchestrator) = state.team_orchestrator(&task.name) else {
            continue;
        };
        let to = orchestrator.id.clone();
        if state
            .messages
            .iter()
            .any(|m| m.to == to && m.from == store::SCHEDULE)
        {
            continue;
        }
        let message = Message {
            from: store::SCHEDULE.into(),
            to,
            text: task.prompt.clone(),
        };
        fired.push(task.name.clone());
        state.messages.push(message);
        state.tasks[i].last_run = now;
    }
    fired
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::store::{Agent, Task};

    /// Monday 2026-10-26 09:30.
    fn monday() -> LocalMinute {
        LocalMinute {
            minute: 30,
            hour: 9,
            day: 26,
            month: 10,
            weekday: 1,
        }
    }

    fn matches(schedule: &str, at: LocalMinute) -> bool {
        Schedule::parse(schedule).unwrap().matches(&at)
    }

    #[test]
    fn parsing_accepts_every_field_form() {
        for text in [
            "* * * * *",
            "0 3 * * *",
            "0-59/15 0-23 1-31 1-12 0-7",
            "*/5 */2 */10 */3 */2",
            "1,2,10-20/5,*/30 9 1,15 6 1-5",
            "  59 23 31 12 7  ",
        ] {
            Schedule::parse(text).unwrap_or_else(|e| panic!("{text}: {e}"));
        }
    }

    #[test]
    fn parsing_rejects_bad_fields() {
        for (text, error) in [
            ("60 * * * *", "minute 60 is outside 0-59"),
            ("* 24 * * *", "hour 24 is outside 0-23"),
            ("* * 0 * *", "day-of-month 0 is outside 1-31"),
            ("* * * 13 *", "month 13 is outside 1-12"),
            ("* * * * 8", "day-of-week 8 is outside 0-7"),
            ("1, * * * *", "invalid minute field \"1,\""),
            ("*/0 * * * *", "invalid minute field \"*/0\""),
            ("5/2 * * * *", "invalid minute field \"5/2\""),
            ("* 9-5 * * *", "invalid hour field \"9-5\""),
            ("* * * * mon", "invalid day-of-week field \"mon\""),
            ("* * * *", "five fields"),
            ("* * * * * *", "five fields"),
            ("@hourly", "five fields"),
            ("", "five fields"),
        ] {
            let err = Schedule::parse(text).unwrap_err().to_string();
            assert!(err.contains(error), "{text:?}: {err}");
        }
    }

    #[test]
    fn matching_follows_each_field() {
        let at = monday();
        assert!(matches("* * * * *", at));
        assert!(matches("30 9 26 10 1", at));
        assert!(matches("*/15 * * * *", at));
        assert!(!matches("*/20 * * * *", at));
        assert!(matches("0-40/10 * * * *", at));
        assert!(!matches("0-20/10 * * * *", at));
        assert!(matches("29-31 8-10 * * *", at));
        assert!(matches("1,30,45 * * * *", at));
        assert!(!matches("1,31,45 * * * *", at));
        assert!(!matches("* * * 11 *", at));
        assert!(matches("* * * * 1-5", at));
        assert!(!matches("* * * * 2-6", at));
    }

    #[test]
    fn sunday_is_both_0_and_7() {
        let sunday = LocalMinute {
            weekday: 0,
            day: 25,
            ..monday()
        };
        assert!(matches("* * * * 0", sunday));
        assert!(matches("* * * * 7", sunday));
        assert!(matches("* * * * 5-7", sunday));
        assert!(!matches("* * * * 7", monday()));
    }

    #[test]
    fn a_day_matching_either_restricted_day_field_matches() {
        // Both restricted: the 1st or a Monday.
        assert!(matches("* * 1 * 1", monday()));
        let first = LocalMinute {
            day: 1,
            weekday: 4,
            ..monday()
        };
        assert!(matches("* * 1 * 1", first));
        let other = LocalMinute {
            day: 2,
            weekday: 5,
            ..monday()
        };
        assert!(!matches("* * 1 * 1", other));
        // With one day field unrestricted, both must match.
        assert!(!matches("* * 1 * *", monday()));
        assert!(!matches("* * */2 * 2", monday()));
        assert!(matches("* * */1 * 1", monday()));
    }

    #[test]
    fn local_minutes_come_from_the_local_time() {
        // Any time's weekday follows the day before's.
        let now = unix_now();
        let (today, yesterday) = (LocalMinute::at(now), LocalMinute::at(now - 86400));
        assert_eq!(today.weekday, (yesterday.weekday + 1) % 7);
        assert!(today.minute < 60 && today.hour < 24);
        assert!((1..=31).contains(&today.day) && (1..=12).contains(&today.month));
    }

    fn agent(id: &str, role: &str, team: &str, activity: &str) -> Agent {
        Agent {
            id: id.into(),
            name: id.into(),
            role: role.into(),
            team: team.into(),
            activity: activity.into(),
            ..Agent::default()
        }
    }

    fn task(name: &str, schedule: &str) -> Task {
        Task {
            name: name.into(),
            schedule: schedule.into(),
            prompt: format!("run {name}"),
            last_run: 0,
        }
    }

    #[test]
    fn a_matching_idle_task_queues_its_prompt_once() {
        let mut s = State {
            agents: vec![
                agent("o", store::ORCHESTRATOR, store::DEFAULT_TEAM, store::BUSY),
                agent("n", store::ORCHESTRATOR, "nightly", store::IDLE),
                agent("h", store::ORCHESTRATOR, "hourly", store::IDLE),
            ],
            tasks: vec![task("nightly", "30 9 * * *"), task("hourly", "0 * * * *")],
            ..State::default()
        };
        assert_eq!(fire(&mut s, &monday(), 100), ["nightly"]);
        assert_eq!(
            s.messages,
            [Message {
                from: store::SCHEDULE.into(),
                to: "n".into(),
                text: "run nightly".into(),
            }]
        );
        assert_eq!((s.tasks[0].last_run, s.tasks[1].last_run), (100, 0));
        // The firing still queued keeps the next one from queueing.
        assert!(fire(&mut s, &monday(), 160).is_empty());
        assert_eq!(s.messages.len(), 1);
        assert_eq!(s.tasks[0].last_run, 100);
        // A message from another sender does not.
        s.messages[0].from = "o".into();
        assert_eq!(fire(&mut s, &monday(), 220), ["nightly"]);
        assert_eq!(s.messages.len(), 2);
    }

    #[test]
    fn running_unparsable_and_orphaned_tasks_are_skipped() {
        let mut s = State {
            agents: vec![
                agent("b", store::ORCHESTRATOR, "busy", store::IDLE),
                agent("bw", store::WORKER, "busy", store::BUSY),
                agent("x", store::ORCHESTRATOR, "exited", store::IDLE),
                agent("xw", store::WORKER, "exited", store::EXITED),
                agent("u", store::ORCHESTRATOR, "unparsable", store::IDLE),
            ],
            tasks: vec![
                task("busy", "* * * * *"),
                task("exited", "* * * * *"),
                task("unparsable", "* * *"),
                task("orphan", "* * * * *"),
            ],
            ..State::default()
        };
        // A task whose only worker has exited still fires.
        assert_eq!(fire(&mut s, &monday(), 100), ["exited"]);
        assert_eq!(s.messages.len(), 1);
        assert_eq!(s.messages[0].to, "x");
    }
}
