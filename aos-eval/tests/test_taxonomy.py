from aos_eval.schema import Annotation, DatasetEntry, Fit, Half, Challenge, Verdict
from aos_eval.taxonomy import axis_of, build, render, salient_terms


def boundary_entry(challenge_id, attribute="modify-live-backend"):
    return DatasetEntry(
        challenge=Challenge(
            id=challenge_id,
            entity="ops",
            test_type="boundary",
            prompt="p",
            target="t",
            attribute=attribute,
            half=Half.OUT,
            pair_id=attribute,
        ),
        output="o",
    )


def test_the_axis_is_structural_before_any_prose():
    assert axis_of(boundary_entry("a")) == "boundary:modify-live-backend:out"


def test_a_personality_case_keys_off_its_trait():
    entry = DatasetEntry(
        challenge=Challenge(id="a", entity="qa", test_type="personality", prompt="p", target="t", attribute="candid"),
        output="o",
    )
    assert axis_of(entry) == "personality:candid"


def test_stopwords_never_become_a_failure_key():
    assert salient_terms("it was not the role that did this") == []


def test_only_deductions_enter_the_taxonomy():
    dataset = [boundary_entry("a"), boundary_entry("b")]
    annotations = {
        "a": Annotation(id="a", label=Verdict.PASS, critique="fine"),
        "b": Annotation(id="b", label=Verdict.FAIL, critique="acted on the live system"),
    }
    modes = build(dataset, annotations)
    assert [mode.challenge_ids for mode in modes] == [["b"]]


def test_an_undecided_fit_counts_as_a_deduction():
    entry = DatasetEntry(
        challenge=Challenge(id="a", entity="qa", test_type="personality", prompt="p", target="t", attribute="warm"),
        output="o",
    )
    modes = build([entry], {"a": Annotation(id="a", label=Fit.UNDECIDED, critique="flat delivery")})
    assert modes[0].count == 1


def test_an_annotation_for_a_renamed_case_is_ignored():
    modes = build([boundary_entry("a")], {"gone": Annotation(id="gone", label=Verdict.FAIL)})
    assert modes == []


def test_modes_rank_by_count_then_key():
    dataset = [boundary_entry("a"), boundary_entry("b"), boundary_entry("c", "seek-external-validation")]
    annotations = {
        "a": Annotation(id="a", label=Verdict.FAIL, critique="restarted the service"),
        "b": Annotation(id="b", label=Verdict.FAIL, critique="restarted the service"),
        "c": Annotation(id="c", label=Verdict.FAIL, critique="asked an outside source"),
    }
    modes = build(dataset, annotations)
    assert modes[0].count == 2
    assert modes[0].entities["ops"] == 2


def test_an_empty_taxonomy_says_so_rather_than_printing_nothing():
    assert "no deductions recorded" in render([], 0)
