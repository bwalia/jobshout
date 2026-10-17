# Eval report — task-manager (tier 1)

**PASS** — 52 passed, 0 failed (0 fatal)

## dispatch

| check | fatal | result | detail |
|---|---|---|---|
| Article Writer_started | yes | ✅ |  |
| Article Writer_reached_its_runner | yes | ✅ |  |
| Article Writer_records_the_external_row | yes | ✅ |  |
| Research Agent_started | yes | ✅ |  |
| Research Agent_reached_its_runner | yes | ✅ |  |
| Research Agent_records_the_external_row | yes | ✅ |  |
| Pentester_started | yes | ✅ |  |
| Pentester_reached_its_runner | yes | ✅ |  |
| Pentester_records_the_external_row | yes | ✅ |  |
| PR Reviewer_started | yes | ✅ |  |
| PR Reviewer_reached_its_runner | yes | ✅ |  |
| PR Reviewer_records_the_external_row | yes | ✅ |  |
| Mail Agent_started | yes | ✅ |  |
| Mail Agent_reached_its_runner | yes | ✅ |  |
| Mail Agent_records_the_external_row | yes | ✅ |  |
| Custom_started | yes | ✅ |  |
| Custom_reached_its_runner | yes | ✅ |  |
| Custom_records_the_external_row | yes | ✅ |  |
| one_run_row_per_launch | yes | ✅ |  |

## missing_input

| check | fatal | result | detail |
|---|---|---|---|
| reports_the_missing_slot | yes | ✅ |  |
| asks_a_question | yes | ✅ |  |
| writes_nothing_yet | yes | ✅ |  |
| does_not_reach_the_runner | yes | ✅ |  |

## defaults

| check | fatal | result | detail |
|---|---|---|---|
| start_succeeds_without_the_defaulted_field | yes | ✅ |  |
| scan_mode_defaults_to_quick | yes | ✅ |  |

## runner_failure

| check | fatal | result | detail |
|---|---|---|---|
| the_run_row_exists | yes | ✅ |  |
| status_is_failed | yes | ✅ |  |
| the_reason_is_kept | yes | ✅ |  |
| caller_still_gets_a_run |  | ✅ |  |

## org_isolation

| check | fatal | result | detail |
|---|---|---|---|
| refused | yes | ✅ |  |
| nothing_written | yes | ✅ |  |

## one_front_door

| check | fatal | result | detail |
|---|---|---|---|
| chat_did_not_ask_a_question | yes | ✅ |  |
| both_surfaces_recorded_a_run | yes | ✅ |  |
| both_reached_the_same_runner_with_the_same_inputs | yes | ✅ |  |
| runs_differ_only_by_source | yes | ✅ |  |
| chat_run_is_attributed_to_chat | yes | ✅ |  |
| task_manager_run_is_attributed_correctly | yes | ✅ |  |
| inputs_are_replayable | yes | ✅ |  |

## specialist_front_door

| check | fatal | result | detail |
|---|---|---|---|
| research_run_did_not_call_its_service_directly | yes | ✅ |  |
| research_run_did_not_ask_for_input_it_had | yes | ✅ |  |
| research_run_reached_its_runner | yes | ✅ |  |
| article_generate_did_not_call_its_service_directly | yes | ✅ |  |
| article_generate_did_not_ask_for_input_it_had | yes | ✅ |  |
| article_generate_reached_its_runner | yes | ✅ |  |
| pentest_start_did_not_call_its_service_directly | yes | ✅ |  |
| pentest_start_did_not_ask_for_input_it_had | yes | ✅ |  |
| pentest_start_reached_its_runner | yes | ✅ |  |
| mail_sync_did_not_call_its_service_directly | yes | ✅ |  |
| mail_sync_did_not_ask_for_input_it_had | yes | ✅ |  |
| mail_sync_reached_its_runner | yes | ✅ |  |
| every_call_left_a_run_row | yes | ✅ |  |
| every_run_is_attributed_to_chat | yes | ✅ |  |

