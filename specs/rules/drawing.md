# Drawing

The rules behind [the Drawing overview](../drawing.md). Area code: `drawing`.

## A flow

**R-drawing-01** A stage is drawn below every stage it waits for, by the flow package's own rule, stages that run at the same time are drawn side by side, and no arrow is drawn from a stage it waits for through another. *Test:* `drawing_flow_stage_follows_what_it_waits_for`, `drawing_flow_waits_by_the_flow_rule`

## A statechart

**R-drawing-02** A transition to a state in a row below or in its own row is an arrow with a chip giving its event, its condition as written and a timer's time, one arrow for all the rules between two states; a timer's arrow is dashed; a transition to a state in a row above or the same state is written in its card with no arrow; a state's ticket and timers are written in it, and a final state that fails is marked as failed. *Test:* `drawing_statechart_arrows_name_event_condition_and_timer`

**R-drawing-05** A `send:` to an address is drawn as a card of its own below the state, named by the address, with a dashed arrow labelled with the event. *Test:* `drawing_statechart_send_is_a_card`

## An agent

**R-drawing-06** An agent is drawn as its model, a holder of its tools, with each toolkit a holder of its tools, and `reply`, one below another, with two arrows; Corbie's own tools are not drawn. *Test:* `drawing_agent_is_a_loop_of_model_and_tools`

**R-drawing-10** A toolkit or a tool with `when:` is drawn with a dashed border and its condition in its tooltip, and a card whose job has `approve: true` says so, in a machine of any type. *Test:* `drawing_conditions_approvals_and_calls_are_marked`

## A decision

**R-drawing-11** A decision is drawn as its model, by its name, above its answers, which name each question, joined by one arrow. *Test:* `drawing_decision_is_its_model_and_its_questions`

## A machine inside a machine

**R-drawing-07** A card whose job names a machine shows the machine's type and name, known from its file on the Machines page and from the definition the run was started from on a run's page. On the Machines page it is a link that unfolds the machine's drawing inside the card, at any depth, and the same card then folds it; a job whose machine is an expression has no link. On a run's page it links to the run the job started, an ask's card to the run that ask started. *Test:* `drawing_machine_unfolds_in_its_card`

**R-drawing-09** A job that sends is a card giving its event and its address, and a job that calls is a card giving the location it calls; on a run's page a call's card links to the run it called, and on the Machines page to the file of the machine at the location it writes out. A stage waits for each stage its address and its data read. *Test:* `drawing_call_and_send_jobs_are_cards`, `drawing_conditions_approvals_and_calls_are_marked`, `drawing_call_links_to_the_machine_it_calls`

## A run on the drawing

**R-drawing-03** A run is marked on its drawing: a flow's stages by their status, and as held where the job waits for approval; a statechart's states as current, held where the job waits for approval, entered before or never entered, a state passed through within one step being entered; and an agent's model, and each tool and ask as going, held, used at an earlier step or never used; a run that ended is marked by how it ended, a failed run at the states it ended in and at the card of the job whose failure ended it. *Test:* `drawing_run_is_marked_on_the_drawing`, `drawing_conditions_approvals_and_calls_are_marked`

**R-drawing-08** On a run's page, a toolkit of an agent whose tools the run has not asked for by the step drawn is folded to its name and how many tools it has, and the folded toolkits are drawn one below another after the others; on the Machines page every toolkit is drawn in full. *Test:* `drawing_unused_toolkit_is_folded`

**R-drawing-04** The transition the step drawn took is marked on its arrow or in its card, and no other. *Test:* `drawing_transition_just_taken_is_marked`

## The camera

**R-drawing-12** A drawing is seen through a camera that frames the card the page focuses, or else the cards a run marks as going, held or failed, the deepest unfolded first, with the machine they are in where it can be read at half size or more, or else the whole machine; it never scales the drawing larger than its size to frame it, and it glides to frame again when the graph or the focus changes, until a person pans or zooms, after which Follow gives it back. *Test:* `drawing_camera_fits_the_whole_machine`, `drawing_camera_follows_the_run`, `drawing_camera_frames_the_card_focused`, `drawing_camera_is_moved_by_hand`

**R-drawing-13** On a run's page, the drawing of a run a job started, as the page gives it, is drawn inside that job's card with its own marks. *Test:* `drawing_child_run_is_inside_its_card`, `drawing_camera_follows_the_run`

**C-drawing-01** What a card, a chip and a holder write fits inside it in the fonts the page is given, and no two cards overlap: the layout estimates the widths of letters, and only a browser sets them. *Test:* `drawing_text_fits_its_card`, `drawing_cards_do_not_overlap`

## Interfaces

Drawing uses no other area of the UI. Runs and Machines give a page a machine's graph, from `Draw` and `Unfolded`, with the graphs of a run's children in `Run.In`, and the page draws it with `Drawing` from `drawing.js`, which the UI serves at `/drawing/`, giving it the path of a card to frame where it focuses one.
