#!/usr/bin/env python3
"""Offline regression tests for scripts/sweep/runner.py scoring/parsing.

Run: python3 scripts/sweep/test_runner.py  (stdlib only, no pytest needed)
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from runner import (  # noqa: E402
    EXTRA_BODY_ALLOWLIST,
    REASONING_MODEL_DEFAULTS,
    call_model,
    parse_lines,
    reasoning_effort_for,
    score_task,
)


def test_parse_lines_basic():
    t = parse_lines("CLAIM: C-14\nSTATUS: REPAIRED\nEVIDENCE_COUNT: 3", ["CLAIM", "STATUS", "EVIDENCE_COUNT"])
    assert t == {"CLAIM": "C-14", "STATUS": "REPAIRED", "EVIDENCE_COUNT": "3"}, t


def test_parse_lines_strips_value_whitespace():
    t = parse_lines("DATE:   2025-11-03  \nSOURCE:\tS-17", ["DATE", "SOURCE"])
    assert t == {"DATE": "2025-11-03", "SOURCE": "S-17"}, t


def test_parse_lines_ignores_non_matching():
    t = parse_lines("CLAIMANT: X\nnote about CLAIM\nCLAIM: C-14", ["CLAIM"])
    assert t == {"CLAIM": "C-14"}, t


def test_parse_lines_fallback_bare_values_positional():
    # Fire-sweep 2026-08-05 finding #1: models drop DATE:/SOURCE: prefix
    # but emit correct values in expected order. Fallback positional parse.
    t = parse_lines("2025-11-03\nS-17", ["DATE", "SOURCE"])
    assert t == {"DATE": "2025-11-03", "SOURCE": "S-17"}, t


def test_parse_lines_fallback_no_partial_mixed():
    # When some keys found by prefix, no fallback attempted (mixed format
    # is treated as intentional, not a non-compliant response).
    t = parse_lines("DATE: 2025-11-03\nS-17", ["DATE", "SOURCE"])
    assert t == {"DATE": "2025-11-03"}, t
    assert "SOURCE" not in t


def test_parse_lines_fallback_wrong_count_no_match():
    # Line count != key count: no fallback (avoids false positives on prose).
    t = parse_lines("just one line", ["DATE", "SOURCE"])
    assert t == {}, t


def test_parse_lines_fallback_three_fields():
    t = parse_lines("C-14\nREPAIRED\n3", ["CLAIM", "STATUS", "EVIDENCE_COUNT"])
    assert t == {"CLAIM": "C-14", "STATUS": "REPAIRED", "EVIDENCE_COUNT": "3"}, t


def test_parse_lines_empty_and_truncated():
    assert parse_lines("", ["DATE", "SOURCE"]) == {}
    assert parse_lines("DATE: 2025-", ["DATE", "SOURCE"]) == {"DATE": "2025-"}
    # no line starting with SOURCE -> missing key
    assert "SOURCE" not in parse_lines("DATE: 2025-11-03", ["DATE", "SOURCE"])


def test_parse_lines_last_occurrence_wins():
    # Document behavior: later duplicate key overwrites earlier one.
    t = parse_lines("A: 1\nA: 2", ["A"])
    assert t == {"A": "2"}, t


def _task(expected):
    keys = ",".join(k.upper() for k in expected)
    return {"scoring": f"exact_lines:{keys}", "expected": expected}


def test_score_exact_match():
    ok, fields, _ = score_task(_task({"claim": "C-14", "status": "REPAIRED"}),
                            "CLAIM: C-14\nSTATUS: REPAIRED")
    assert ok and all(f["ok"] for f in fields.values()), fields


def test_score_truncated_fails_field():
    # extract-date-reinforced truncation case: "DATE: 2025-11" without day
    ok, fields, _ = score_task(_task({"date": "2025-11-03", "source": "S-17"}),
                            "DATE: 2025-11")
    assert not ok
    assert fields["DATE"] == {"expected": "2025-11-03", "got": "2025-11", "ok": False}
    assert fields["SOURCE"]["got"] is None and not fields["SOURCE"]["ok"]


def test_score_bare_value_fallback_recovers_semantic():
    # Fire-sweep finding: llama-3.1-8b emits bare values without DATE:/SOURCE:
    # prefix but the semantic data is correct. Fallback should recover it.
    ok, fields, _ = score_task(_task({"date": "2025-11-03", "source": "S-17"}),
                            "2025-11-03\nS-17")
    assert ok, fields
    assert fields["DATE"]["ok"] is True
    assert fields["SOURCE"]["ok"] is True


def test_score_empty_response_fails_all():
    ok, fields, _ = score_task(_task({"date": "2025-11-03", "source": "S-17"}), "")
    assert not ok
    assert all(not f["ok"] and f["got"] is None for f in fields.values())


def test_score_bare_value_wrong_count_fails():
    # Bare values with wrong count should not produce false positives.
    ok, _, _ = score_task(_task({"date": "2025-11-03", "source": "S-17"}),
                      "2025-11-03")
    assert not ok


def test_score_bare_value_wrong_order_fails():
    # Bare values in wrong order must fail (positional assignment).
    ok, fields, _ = score_task(_task({"date": "2025-11-03", "source": "S-17"}),
                            "S-17\n2025-11-03")
    assert not ok
    assert fields["DATE"]["got"] == "S-17"
    assert fields["SOURCE"]["got"] == "2025-11-03"


def test_score_case_sensitive_values():
    ok, fields, _ = score_task(_task({"status": "REPAIRED"}), "STATUS: repaired")
    assert not ok and fields["STATUS"]["got"] == "repaired"


def test_score_extra_lines_ignored():
    ok, _, _ = score_task(_task({"date": "2025-11-03", "source": "S-17"}),
                       "I think...\nDATE: 2025-11-03\nSOURCE: S-17\n(trailing)")
    assert ok


def test_score_unknown_scoring_fails_closed():
    ok, fields, _ = score_task({"scoring": "contains:foo", "expected": {}}, "foo")
    assert not ok and "error" in fields


def test_reasoning_effort_explicit_override_wins():
    assert reasoning_effort_for("groq", "qwen/qwen3.6-27b", "low") == "low"
    assert reasoning_effort_for("groq", "llama-3.3-70b-versatile", "none") == "none"


def test_reasoning_effort_defaults_for_known_reasoning_models():
    assert reasoning_effort_for("groq", "qwen/qwen3.6-27b", None) == "none"
    assert reasoning_effort_for("groq", "openai/gpt-oss-20b", None) == "low"
    assert reasoning_effort_for("groq", "openai/gpt-oss-120b", None) == "low"


def test_reasoning_effort_none_for_plain_models():
    assert reasoning_effort_for("groq", "llama-3.3-70b-versatile", None) is None
    assert reasoning_effort_for("groq", "llama-3.1-8b-instant", None) is None
    assert reasoning_effort_for("nvidia_nim", "meta/llama-3.1-8b-instruct", None) is None


def test_reasoning_defaults_have_allowlisted_values():
    # Fail-closed contract: defaults may only use keys from the allowlist.
    assert "reasoning_effort" in EXTRA_BODY_ALLOWLIST
    for prefix, effort in REASONING_MODEL_DEFAULTS.items():
        assert isinstance(prefix, str) and ":" in prefix
        assert effort in {"none", "low", "medium", "high"}, (prefix, effort)


def _load_tasks_manifest():
    import json

    path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "tasks.json")
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


def test_tasks_manifest_ids_unique_and_scoring_consistent():
    doc = _load_tasks_manifest()
    assert doc["schema_version"] == 1
    tasks = doc["tasks"]
    ids = [t["id"] for t in tasks]
    assert len(ids) == len(set(ids)), ("duplicate task ids", ids)
    for t in tasks:
        assert t["scoring"].startswith("exact_lines:"), t["id"]
        keys = t["scoring"].split(":", 1)[1].split(",")
        assert sorted(k.lower() for k in keys) == sorted(t["expected"].keys()), t["id"]
        assert t["operation"] in {"EXTRACT", "SYNTHESIZE", "CONFLICT", "REPAIR"}, t["id"]
        assert t["format"] == "DELIMITED", t["id"]


def test_tasks_manifest_every_expected_key_appears_in_prompt_header():
    # Guard rail: each scored line label must be stated literally in the
    # prompt as "KEY:" so weak models see the wire format, not a paraphrase.
    doc = _load_tasks_manifest()
    for t in doc["tasks"]:
        keys = t["scoring"].split(":", 1)[1].split(",")
        for k in keys:
            assert (k + ":") in t["prompt"], (t["id"], k)


def test_tasks_manifest_offline_oracle_self_scores_perfectly():
    # The oracle response built from expected values must score 100% per task;
    # catches expected/scoring drift without any live call.
    doc = _load_tasks_manifest()
    for t in doc["tasks"]:
        keys = t["scoring"].split(":", 1)[1].split(",")
        oracle = "\n".join(k + ": " + str(t["expected"][k.lower()]) for k in keys)
        ok, fields, _ = score_task(t, oracle)
        assert ok, (t["id"], fields)


def test_multi_factor_superset_wrong_answer_fails_both_fields():
    # Live failure mode catalogued 2026-08-01 22:25: both llama-3.1-8b
    # deployments deterministically answered "F-3,F-4" (listing a satisfied
    # factor alongside the failed one) instead of "F-4". The scorer must
    # reject the superset; it is a semantic filtering error, not a format
    # error, so FACTORS mismatches and the trial is wrong.
    doc = _load_tasks_manifest()
    for tid in ("synthesize-multi-factor", "synthesize-multi-factor-restated"):
        t = next(x for x in doc["tasks"] if x["id"] == tid)
        ok, fields, _ = score_task(t, "VERDICT: FAIL\nFACTORS: F-3,F-4")
        assert not ok, (tid, fields)
        assert fields["VERDICT"]["ok"] is True
        assert fields["FACTORS"]["ok"] is False
        assert fields["FACTORS"]["got"] == "F-3,F-4"


def test_multi_factor_order_and_duplicates_are_scored_strictly():
    # ascending order is part of the oracle string; reorder or duplicate
    # ids must not silently pass (guard against scorer leniency drift).
    doc = _load_tasks_manifest()
    t = next(x for x in doc["tasks"] if x["id"] == "synthesize-multi-factor")
    for bad in ("VERDICT: FAIL\nFACTORS: F-4,F-4", "VERDICT: FAIL\nFACTORS: "):
        ok, fields, _ = score_task(t, bad)
        assert not ok, (bad, fields)
    ok, _, _ = score_task(t, "VERDICT: FAIL\nFACTORS: F-4")
    assert ok


def test_factor_trace_per_line_scoring_isolates_the_gap():
    # synthesize-factor-trace decomposes the judgement: a model can get the
    # per-factor lines right and still fail the final filtering line. The
    # scorer must expose that split so analysis can separate comprehension
    # (F lines) from negation filtering (FACTORS).
    doc = _load_tasks_manifest()
    t = next(x for x in doc["tasks"] if x["id"] == "synthesize-factor-trace")
    ok, fields, _ = score_task(
        t, "F3: OK\nF4: FAIL\nF5: OK\nVERDICT: FAIL\nFACTORS: F-3,F-4")
    assert not ok
    per_line_ok = all(fields[k]["ok"] for k in ("F3", "F4", "F5", "VERDICT"))
    assert per_line_ok, fields
    assert fields["FACTORS"]["ok"] is False
    # full oracle passes
    oracle = "F3: OK\nF4: FAIL\nF5: OK\nVERDICT: FAIL\nFACTORS: F-4"
    ok_all, _, _ = score_task(t, oracle)
    assert ok_all


def test_call_model_rejects_non_allowlisted_extra_body():
    import types

    prov = {"base_url": "http://127.0.0.1:1", "api_key_env": "DUMMY_KEY_FOR_TEST"}
    os.environ["DUMMY_KEY_FOR_TEST"] = "x"
    budget = [10.0]
    r = call_model(prov, "m", "p", 0.0, 16, 1, budget, 0,
                   extra_body={"stream": True})
    assert r["ok"] is False
    assert r["error_class"] == "config", r
    assert "allowlist" in r["error_body"]
    # function must be stdlib-only importable; stay defensive about its type
    assert isinstance(call_model, types.FunctionType)


def run_all():
    g = dict(globals())
    tests = sorted(n for n in g if n.startswith("test_"))
    failed = 0
    for name in tests:
        try:
            g[name]()
            print(f"PASS {name}")
        except AssertionError as e:
            failed += 1
            print(f"FAIL {name}: {e}")
        except Exception as e:  # noqa: BLE001
            failed += 1
            print(f"ERROR {name}: {e!r}")
    print(f"{len(tests) - failed}/{len(tests)} passed")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(run_all())
