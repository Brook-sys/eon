from pathlib import Path

p=Path('internal/kernel/model_executor_multiturn_test.go')
s=p.read_text()

# We need to change the final proposal JSON in the dummy tool output to something that changeset parser accepts
# It needs a real `Kind`, `EntityType`, `EntityID`

s = s.replace('Text: `{"changes":[{"kind":"ADD","entity_type":"observation","entity_id":"obs_final","payload_ref":"payload"}]}`', 'Text: `{"changes":[{"type":"add","entity_type":"observation","entity_id":"obs_final","payload_ref":"payload"}]}`')
s = s.replace('Text: `{"changes":[{"type":"replace","path":"/result.md","content":"Weather is sunny, hotels are nice."}]}`', 'Text: `{"changes":[{"type":"add","entity_type":"observation","entity_id":"obs_final","payload_ref":"payload"}]}`')

p.write_text(s)
