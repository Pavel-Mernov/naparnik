package tools

import "github.com/worker4556/naparnik/backend/internal/store"

type accessPlanStep struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type accessRule struct {
	SystemView
	Step *accessPlanStep `json:"step,omitempty"`
}

// SC-33, BR-45: объясняем по каталогу, не обращаясь к закрытым страницам базы.
// Срок по плану не запрещает раннюю заявку: её согласует руководитель.
func getAccessRules(c Ctx, args map[string]any) Result {
	id := str(args, "system")
	if id != "" {
		if e := checkAccess(c, id); e != nil {
			return Result{Error: e}
		}
	}
	systems := []accessRule{}
	for _, system := range requestableSystems(c) {
		if id == "" || system.System == id {
			systems = append(systems, accessRule{system, relatedAccessStep(c, system.System)})
		}
	}
	return ok(map[string]any{
		"week": c.Store.Week(c.User), "systems": systems,
		"approver":            map[string]string{"id": c.User.LeadID, "name": c.Store.Name(c.User.LeadID)},
		"self_service_in_app": SelfServiceHint, "label": TestInstructionLabel,
	})
}

// BR-40: ссылка только на существующий шаг собственного плана, а не чужой или весь план.
func relatedAccessStep(c Ctx, system string) *accessPlanStep {
	for _, state := range c.Store.Data.Access.CurrentState[c.User.ID] {
		if state.System != system || state.RelatedStep == "" {
			continue
		}
		for _, step := range planViews(c, c.User.ID) {
			if step.ID == state.RelatedStep && step.Owner == store.RoleIntern {
				return &accessPlanStep{ID: step.ID, Title: step.Title}
			}
		}
	}
	return nil
}
