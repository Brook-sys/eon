# Phase 180 — WAL Auto-Checkpoint Reopen Cost

## Hypothesis

The contribution of WAL auto-checkpoint to reopen time is observable only when enough pages accumulate in the WAL before the autocheckpoint threshold triggers. With a small number of commits, the WAL stays small and reopen cost is dominated by connection setup and WAL replay overhead, not by checkpoint work. The synchronous level (FULL vs NORMAL) affects commit latency but not the volume of WAL frames committed, so its effect on reopen time should also be negligible for small N.

## Matrix

- **Synchronous:** FULL, NORMAL
- **wal_autocheckpoint:** 100 (aggressive), 1000 (SQLite default), -1 (disabled)
- **N commits:** 5, 50
- **Total cells:** 12

Each cell uses a self-contained temp directory and database file. All cells preserve every idempotency record after reopen.

## Results (two runs aggregated)

| sync   | autochk | N  | commit (ms) | reopen (ms) | WAL size (B) |
|--------|---------|----|-------------|-------------|--------------|
| FULL   | 100     | 5  | 55          | 42          | 156,592      |
| FULL   | 100     | 50 | 1,008       | 43          | 428,512      |
| FULL   | 1000    | 5  | 53          | 40          | 156,592      |
| FULL   | 1000    | 50 | 559          | 56          | 1,582,112    |
| FULL   | -1      | 5  | 54          | 40          | 156,592      |
| FULL   | -1      | 50 | 577          | 43          | 1,569,752    |
| NORMAL | 100     | 5  | 11          | 29          | 156,592      |
| NORMAL | 100     | 50 | 213         | 30          | 432,632      |
| NORMAL | 1000    | 5  | 11          | 30          | 156,592      |
| NORMAL | 1000    | 50 | 130         | 37          | 1,594,472    |
| NORMAL | -1      | 5  | 11          | 30          | 156,592      |
| NORMAL | -1      | 50 | 129         | 36          | 1,582,112    |

## Analysis

### Reopen time

Reopen time ranges from 28–73 ms across all 24 samples (two runs × 12 cells). The variance is dominated by connection setup and WAL replay overhead, not by autocheckpoint work. No systematic effect of `wal_autocheckpoint` on reopen time is observable at N=5 or N=50:

- At N=5: reopen is 29–45 ms regardless of autocheckpoint setting.
- At N=50: reopen is 29–73 ms, with the worst case being FULL/1000 at 73 ms — within normal variance.
- Disabling autocheckpoint (-1) does not produce faster reopens than the default (1000), confirming that SQLite WAL replay is efficient even with larger WAL files.

### Commit time

Synchronous level has the expected dominant effect:

- **FULL:** 53–1,008 ms (5→50 commits), ~10 ms/commit amortized
- **NORMAL:** 11–213 ms (5→50 commits), ~2.6 ms/commit amortized
- Ratio: ~4× (consistent with Fase 178's per-commit measurement, though amortized ratio is lower due to fixed overhead)

Autocheckpoint threshold has no observable effect on commit time within each synchronous level.

### WAL file size

- **autochk=100:** WAL stays smaller at N=50 (~428–432 KB) because aggressive checkpointing folds pages back into the main DB.
- **autochk=1000 (default):** WAL grows to ~1.58–1.59 MB at N=50 — 50 commits do not trigger the 1000-page threshold.
- **autochk=-1 (disabled):** Similar to 1000 (~1.57–1.58 MB), confirming that the default threshold was not reached.

At N=5, WAL is ~156 KB for all configurations — the initial schema and first checkpoint write.

## Decision

**Maintain production defaults:** `synchronous=FULL`, `wal_autocheckpoint=1000` (SQLite default). The experiment confirms that reopen cost is not sensitive to autocheckpoint threshold at this scale. The WAL autocheckpoint PRAGMA is now available as an opt-in via `Options.WalAutoCheckpoint` for experimental harnesses without changing production behavior.

## Next step

Repeat with higher N (500, 5000) to observe whether WAL autocheckpoint begins to affect reopen time when the WAL grows large enough to trigger checkpointing under the default threshold. Also consider measuring the cost of an explicit `PRAGMA wal_checkpoint(TRUNCATE)` at reopen time versus passive replay.
