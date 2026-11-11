package agent

import (
	"context"

	"github.com/worker4556/naparnik/backend/internal/llm"
	"github.com/worker4556/naparnik/backend/internal/mockdata"
	"github.com/worker4556/naparnik/backend/internal/tools"
)

func (a *Agent) ApprovePractice(user *mockdata.User, intern, id, text string, version int) tools.Result {
	if containsPII(text) {
		return tools.Result{Error: &tools.Error{Code: "validation", Message: "Учебное задание должно содержать только синтетические данные."}}
	}
	return tools.ApprovePractice(tools.Ctx{Store: a.Store, User: user}, intern, id, text, version)
}

func (a *Agent) GeneratePractice(ctx context.Context, user *mockdata.User, intern, id string) tools.Result {
	c := tools.Ctx{Store: a.Store, User: user}
	input, needed, e := tools.PreparePractice(c, intern, id)
	if e != nil {
		return tools.Result{Error: e}
	}
	if !needed {
		return tools.PracticeList(c)
	}
	ctx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()
	text, err := llm.GeneratePractice(ctx, a.Provider, input)
	if err != nil || containsPII(text) {
		return tools.Result{Error: &tools.Error{Code: "unavailable", Message: "Не удалось подготовить учебное задание. Практику можно пропустить или повторить подготовку."}}
	}
	return tools.FinishPracticeGeneration(c, intern, id, text)
}

// Черновики появляются при первом входе стажёра/бадди, когда шаг уже есть в утверждённом плане.
// Два одновременных входа могут вызвать LLM дважды, но сохраняется только один черновик.
func (a *Agent) SyncPractices(ctx context.Context, user *mockdata.User) tools.Result {
	c := tools.Ctx{Store: a.Store, User: user}
	list := tools.PracticeList(c)
	if !list.OK {
		return list
	}
	for _, v := range list.Data.([]tools.PracticeView) {
		if v.TaskStatus == "Pending" {
			r := a.GeneratePractice(ctx, user, v.InternID, v.ID)
			if !r.OK {
				return r
			}
		}
	}
	return tools.PracticeList(c)
}

func (a *Agent) SubmitPractice(ctx context.Context, user *mockdata.User, intern, id, key, answer string, version int) tools.Result {
	c := tools.Ctx{Store: a.Store, User: user}
	if containsPII(answer) {
		return tools.Result{Error: &tools.Error{Code: "validation", Message: "Отправь только учебный пример без персональных и банковских данных."}}
	}
	input, previous, e := tools.CheckPracticeAttempt(c, intern, id, key, answer, version)
	if e != nil {
		return tools.Result{Error: e}
	}
	if previous != nil {
		return tools.Result{OK: true, Data: *previous}
	}
	ctx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()
	review, err := llm.ReviewPractice(ctx, a.Provider, input)
	if err == nil {
		for _, c := range review.Checks {
			if containsPII(c.Feedback) {
				err = context.Canceled
			}
		}
	}
	if err != nil {
		return tools.Result{Error: &tools.Error{Code: "unavailable", Message: "Проверка временно недоступна. Попытка не израсходована — повтори отправку."}}
	}
	return tools.CommitPracticeAttempt(c, intern, id, key, answer, version, review)
}
