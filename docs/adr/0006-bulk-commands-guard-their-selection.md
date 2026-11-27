# Bulk commands guard their Selection

`edit` and `hunt` act on a Selection: every Title in one Instance's Library that a query, filters, IDs on stdin or a hand-picked list match. One mistyped filter could otherwise unmonitor a whole Library or send hundreds of searches to the indexers, so these commands guard the Selection in four ways.

- **Nothing without asking.** In a terminal, they show the Selection in a multi-select picker, all preselected, then one summary to confirm. From a script, or when IDs come from stdin (`-`), they never ask: they refuse unless given `--yes`, or `--dry-run`, which prints the plan and changes nothing.
- **No whole Library by default.** With no query, filter or stdin, they refuse unless given `--all`, so leaving out a filter cannot widen the Selection to everything.
- **A cap on Hunts.** A Hunt over more than 25 Titles is refused unless `--max` raises the cap, or `--all` is given, because every Title is a search against the indexers' API limits. `edit` has no cap, since it changes only the Library.
- **One Instance at a time.** Like every write (ADR 0002), they name their Instance; there is no `arr edit` across every Instance.

We chose this over trusting the person typing, as the Apps' own bulk editors do, because a command line has no visible checkboxes and a script repeats a mistake every time it runs. The cost is extra flags in scripts (`--yes`, `--max`), and since those flags are part of the scripting surface (ADR 0004), loosening a guard later is a change scripts may notice.
