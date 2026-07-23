# Sustained ingress recovery-delay matrix

- Schema: `motor-autonomo.sustained-ingress-delay-matrix.v1`
- Hypothesis: A fixed process-level recovery delay may reduce repeated exhaustion without changing transaction retry policy.
- Scenario: Four SQLite processes, six rotating-leader cycles, one durable winner per cycle.

## Results

- 50 ms: convergence 4799 ms; exhaustion rate 0.125 (3/24); attempts 50; fairness [2, 2, 1, 1]; pending [6, 5, 4, 3, 2, 1, 0].
- 100 ms: convergence 4981 ms; exhaustion rate 0.208 (5/24); attempts 53; fairness [2, 2, 1, 1]; pending [6, 5, 4, 3, 2, 1, 0].
- 200 ms: convergence 5601 ms; exhaustion rate 0.208 (5/24); attempts 52; fairness [2, 2, 1, 1]; pending [6, 5, 4, 3, 2, 1, 0].

## Decision

Keep the production delay at 100 ms. All samples converged fairly; the single-sample matrix is insufficient for tuning, and 200 ms increased convergence time without reducing exhaustion versus 100 ms.

## Next experiment

Repeat each delay with at least five isolated runs before considering a production change.
