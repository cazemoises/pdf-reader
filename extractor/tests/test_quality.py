from quality import evaluate
from layout import normalized


def q(text):
    return evaluate([{'text': text}])


def test_wrong_semantics_and_wrong_order_have_high_integrity():
    assert q('Bob owes Alice 100')['score'] == q('Alice owes Bob 100')['score'] == 1
    assert evaluate([{'text': 'Second'}, {'text': 'First'}])['score'] == 1


def test_correct_literal_replacement_character_has_lower_integrity():
    # A document explaining this character can legitimately contain it.
    assert q('The replacement character is �')['score'] < 1


def test_spaces_cannot_dilute_corruption():
    assert q('�'*10+' '*10_000)['invalid_ratio'] == 1
    assert q(' \n\t')['score'] is None
    assert q(' \n\t')['empty']


def test_repetition_is_a_signal_not_proof_of_error():
    result = evaluate([{'text': 'Refrain'}, {'text': 'Refrain'}])
    assert result['duplicate_blocks'] == 1
    assert result['score'] == 1


def test_unicode_controls_and_nul_are_explicit():
    assert q('A\x01B')['invalid_characters'] == 1
    assert normalized('A\x00B') == 'A�B'  # JSONB cannot store U+0000.
    assert q('Ação, café, coração.')['invalid_characters'] == 0
