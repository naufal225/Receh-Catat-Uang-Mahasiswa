package app_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"receh/internal/domain"
	"receh/internal/modules/finance/model"
	"receh/internal/modules/finance/service"
)

// person is a registered user with a known email, for invitations.
type person struct {
	*client
	id    int64
	email string
}

func (e *env) person(name string) person {
	email := fmt.Sprintf("%s%d@test.local", strings.ToLower(name), emailSeq.Add(1))
	c := e.client("")
	c.expect(201, "POST", "/api/auth/register", M{"name": name, "email": email, "password": "password-123"})
	me := must[domain.User](c, 200, "GET", "/api/auth/me", nil)
	return person{client: c, id: me.ID, email: email}
}

func orgsOf(c *client) []domain.OrgMembership {
	return must[[]domain.OrgMembership](c, 200, "GET", "/api/orgs", nil)
}

func (p person) personalOrg() domain.OrgMembership {
	for _, o := range orgsOf(p.client) {
		if o.Personal {
			return o
		}
	}
	p.e.t.Fatal("no personal org")
	return domain.OrgMembership{}
}

func newTeam(c *client, name string) domain.OrgMembership {
	return must[domain.OrgMembership](c, 201, "POST", "/api/orgs", M{"name": name})
}

func invite(owner *client, orgID int64, email, role string) (token string, id int64) {
	res := must[struct {
		Invite domain.OrgInvite `json:"invite"`
		Token  string           `json:"token"`
	}](owner, 201, "POST", fmt.Sprintf("/api/orgs/%d/invitations", orgID), M{"email": email, "role": role})
	return res.Token, res.Invite.ID
}

func join(p person, token string) {
	p.e.t.Helper()
	p.expect(200, "POST", "/api/invitations/"+token+"/accept", nil)
}

func TestEveryAccountGetsPersonalWorkspaceWithStarterData(t *testing.T) {
	e := newEnv(t)
	a := e.person("Ayu")
	list := orgsOf(a.client)
	if len(list) != 1 || !list[0].Personal || list[0].Role != domain.OrgOwner || list[0].OwnerID != a.id {
		t.Fatalf("orgs %+v", list)
	}
	// The finance module seeded its starter categories into that workspace.
	if cats := must[[]model.Category](a.client, 200, "GET", "/api/categories", nil); len(cats) == 0 {
		t.Fatal("no starter categories in the personal workspace")
	}
	// An admin account has no workspace and no user pages.
	e.admin().expect(403, "GET", "/api/orgs", nil)
}

func TestTeamWorkspaceHasItsOwnData(t *testing.T) {
	e := newEnv(t)
	a := e.person("Ayu")
	team := newTeam(a.client, "Kos Mawar")
	if team.Personal || team.Role != domain.OrgOwner {
		t.Fatalf("team %+v", team)
	}
	at := a.in(team.ID)

	if cats := must[[]model.Category](at, 200, "GET", "/api/categories", nil); len(cats) == 0 {
		t.Fatal("new team workspace got no starter categories")
	}
	at.wallet("Kas Kos", 100_000)
	a.wallet("Pribadi BCA", 50_000)

	teamWallets := must[[]model.Wallet](at, 200, "GET", "/api/wallets", nil)
	personal := must[[]model.Wallet](a.client, 200, "GET", "/api/wallets", nil)
	if len(teamWallets) != 1 || teamWallets[0].Name != "Kas Kos" || len(personal) != 1 || personal[0].Name != "Pribadi BCA" {
		t.Fatalf("workspaces leak into each other: team=%v personal=%v", teamWallets, personal)
	}
	// Same wallet name in two workspaces is fine; the dashboard totals differ.
	at.wallet("Pribadi BCA", 1)
	if d := dashboard(at); d.TotalBalance != 100_001 {
		t.Fatalf("team total %d", d.TotalBalance)
	}
	if d := dashboard(a.client); d.TotalBalance != 50_000 {
		t.Fatalf("personal total %d", d.TotalBalance)
	}
}

func TestOutsidersCannotEnterAWorkspace(t *testing.T) {
	e := newEnv(t)
	a, b := e.person("Ayu"), e.person("Budi")
	team := newTeam(a.client, "Rahasia")
	a.in(team.ID).wallet("Kas", 1000)

	// A header is only a request, never a grant.
	b.in(team.ID).expect(403, "GET", "/api/wallets", nil)
	b.in(team.ID).expect(403, "POST", "/api/wallets", walletBody("Nyelip"))
	b.in(team.ID).expect(403, "GET", "/api/dashboard", nil)
	b.in(999_999).expect(403, "GET", "/api/wallets", nil)
	b.expect(403, "GET", fmt.Sprintf("/api/orgs/%d/members", team.ID), nil)
	b.expect(403, "DELETE", fmt.Sprintf("/api/orgs/%d", team.ID), nil)
	if w := must[[]model.Wallet](b.client, 200, "GET", "/api/wallets", nil); len(w) != 0 {
		t.Fatalf("personal workspace shows %v", w)
	}

	req := func(h string) int {
		c := e.client("")
		c.expect(201, "POST", "/api/auth/register", M{"name": "X", "email": fmt.Sprintf("x%d@test.local", emailSeq.Add(1)), "password": "password-123"})
		status, _ := c.doHeader("GET", "/api/wallets", h)
		return status
	}
	if got := req("abc"); got != 400 {
		t.Fatalf("garbage X-Org-Id gave %d", got)
	}
}

func TestInvitationFlowAndRoles(t *testing.T) {
	e := newEnv(t)
	owner, bob, cara := e.person("Ayu"), e.person("Bob"), e.person("Cara")
	team := newTeam(owner.client, "Tim")
	ot := owner.in(team.ID)
	ot.wallet("Kas", 10_000)

	token, _ := invite(owner.client, team.ID, bob.email, "member")

	// The link only opens for the person it was written for.
	cara.expect(404, "GET", "/api/invitations/"+token, nil)
	cara.expect(404, "POST", "/api/invitations/"+token+"/accept", nil)
	if inv := must[domain.OrgInvite](bob.client, 200, "GET", "/api/invitations/"+token, nil); inv.OrgName != "Tim" || inv.Role != domain.OrgMember {
		t.Fatalf("invite info %+v", inv)
	}
	bob.expect(404, "GET", "/api/invitations/nope", nil)

	join(bob, token)
	bob.expect(404, "POST", "/api/invitations/"+token+"/accept", nil) // single use

	// Members see and use the team's data.
	bt := bob.in(team.ID)
	if w := must[[]model.Wallet](bt, 200, "GET", "/api/wallets", nil); len(w) != 1 || w[0].Name != "Kas" {
		t.Fatalf("member sees %v", w)
	}
	bt.wallet("Dompet Bob", 0)
	if members := must[[]domain.OrgPerson](bt, 200, "GET", fmt.Sprintf("/api/orgs/%d/members", team.ID), nil); len(members) != 2 || members[0].Role != domain.OrgOwner {
		t.Fatalf("members %+v", members)
	}

	// A plain member manages nothing.
	base := fmt.Sprintf("/api/orgs/%d", team.ID)
	bob.expect(403, "POST", base+"/invitations", M{"email": cara.email, "role": "member"})
	bob.expect(403, "GET", base+"/invitations", nil)
	bob.expect(403, "PUT", base, M{"name": "Diambil alih"})
	bob.expect(403, "DELETE", base, nil)
	bob.expect(403, "PUT", fmt.Sprintf("%s/members/%d", base, bob.id), M{"role": "admin"})
	bob.expect(403, "POST", base+"/transfer", M{"userId": bob.id})
	bob.expect(409, "DELETE", fmt.Sprintf("%s/members/%d", base, owner.id), nil) // the owner is protected

	// The owner promotes Bob; admins invite members but not other admins.
	owner.expect(204, "PUT", fmt.Sprintf("%s/members/%d", base, bob.id), M{"role": "admin"})
	bob.expect(201, "POST", base+"/invitations", M{"email": cara.email, "role": "member"})
	bob.expect(403, "POST", base+"/invitations", M{"email": "d@test.local", "role": "admin"})
	bob.expect(204, "PUT", base, M{"name": "Tim Baru"})
	bob.expect(403, "DELETE", base, nil) // only the owner deletes

	// The owner can be neither demoted nor removed.
	owner.expect(409, "PUT", fmt.Sprintf("%s/members/%d", base, owner.id), M{"role": "member"})
	bob.expect(409, "DELETE", fmt.Sprintf("%s/members/%d", base, owner.id), nil)
	owner.expect(409, "DELETE", fmt.Sprintf("%s/members/%d", base, owner.id), nil)
	owner.expect(400, "PUT", fmt.Sprintf("%s/members/%d", base, bob.id), M{"role": "owner"})

	// Inviting someone already inside is refused; revoking works.
	owner.expect(409, "POST", base+"/invitations", M{"email": bob.email, "role": "member"})
	pending := must[[]domain.OrgInvite](owner.client, 200, "GET", base+"/invitations", nil)
	if len(pending) != 1 || pending[0].Email != cara.email {
		t.Fatalf("pending %+v", pending)
	}
	owner.expect(204, "DELETE", fmt.Sprintf("%s/invitations/%d", base, pending[0].ID), nil)
	owner.expect(404, "DELETE", fmt.Sprintf("%s/invitations/%d", base, pending[0].ID), nil)
}

func TestLeavingAndRemovalCutAccessAtOnce(t *testing.T) {
	e := newEnv(t)
	owner, bob, cara := e.person("Ayu"), e.person("Bob"), e.person("Cara")
	team := newTeam(owner.client, "Tim")
	for _, p := range []person{bob, cara} {
		token, _ := invite(owner.client, team.ID, p.email, "member")
		join(p, token)
	}
	base := fmt.Sprintf("/api/orgs/%d", team.ID)
	bob.in(team.ID).expect(200, "GET", "/api/wallets", nil) // warms the role cache

	// Admins cannot remove admins; members can leave; the owner removes anyone.
	bob.expect(204, "DELETE", fmt.Sprintf("%s/members/%d", base, bob.id), nil) // leaves
	bob.in(team.ID).expect(403, "GET", "/api/wallets", nil)                    // immediately
	cara.in(team.ID).expect(200, "GET", "/api/wallets", nil)
	owner.expect(204, "DELETE", fmt.Sprintf("%s/members/%d", base, cara.id), nil)
	cara.in(team.ID).expect(403, "GET", "/api/wallets", nil)
	cara.expect(403, "GET", base+"/members", nil)
	if len(orgsOf(cara.client)) != 1 { // only the personal one is left
		t.Fatal("cara still sees the team")
	}
}

func TestOwnershipTransfer(t *testing.T) {
	e := newEnv(t)
	owner, bob := e.person("Ayu"), e.person("Bob")
	team := newTeam(owner.client, "Tim")
	token, _ := invite(owner.client, team.ID, bob.email, "member")
	join(bob, token)
	base := fmt.Sprintf("/api/orgs/%d", team.ID)

	owner.expect(400, "POST", base+"/transfer", M{"userId": owner.id})
	owner.expect(404, "POST", base+"/transfer", M{"userId": 999_999})
	owner.expect(204, "POST", base+"/transfer", M{"userId": bob.id})

	roles := map[int64]domain.OrgRole{}
	for _, m := range must[[]domain.OrgPerson](bob.client, 200, "GET", base+"/members", nil) {
		roles[m.UserID] = m.Role
	}
	if roles[bob.id] != domain.OrgOwner || roles[owner.id] != domain.OrgAdmin {
		t.Fatalf("roles after transfer %v", roles)
	}
	owner.expect(403, "DELETE", base, nil) // no longer the owner
	bob.expect(204, "DELETE", base, nil)
	owner.in(team.ID).expect(403, "GET", "/api/wallets", nil)
	if len(orgsOf(owner.client)) != 1 {
		t.Fatal("deleted team still listed")
	}
}

func TestPersonalWorkspaceIsProtected(t *testing.T) {
	e := newEnv(t)
	a, b := e.person("Ayu"), e.person("Bob")
	p := a.personalOrg()
	base := fmt.Sprintf("/api/orgs/%d", p.ID)
	a.expect(409, "POST", base+"/invitations", M{"email": b.email, "role": "member"})
	a.expect(409, "DELETE", base, nil)
	a.expect(409, "PUT", base, M{"name": "Lain"})
	a.expect(409, "POST", base+"/transfer", M{"userId": b.id})
	a.expect(409, "DELETE", fmt.Sprintf("%s/members/%d", base, a.id), nil)
}

func TestInvitationsExpire(t *testing.T) {
	e := newEnv(t)
	owner, bob := e.person("Ayu"), e.person("Bob")
	team := newTeam(owner.client, "Tim")
	token, _ := invite(owner.client, team.ID, bob.email, "member")
	e.clock.set("2026-01-23") // a week and a bit later
	bob.expect(404, "GET", "/api/invitations/"+token, nil)
	bob.expect(404, "POST", "/api/invitations/"+token+"/accept", nil)
	if p := must[[]domain.OrgInvite](owner.client, 200, "GET", fmt.Sprintf("/api/orgs/%d/invitations", team.ID), nil); len(p) != 0 {
		t.Fatalf("expired invitation still pending: %+v", p)
	}
}

func TestOneInvitationLinkAdmitsOnlyOnce(t *testing.T) {
	e := newEnv(t)
	owner, bob := e.person("Ayu"), e.person("Bob")
	team := newTeam(owner.client, "Tim")
	token, _ := invite(owner.client, team.ID, bob.email, "member")

	var wg sync.WaitGroup
	codes := make([]int, 8)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i], _ = bob.do("POST", "/api/invitations/"+token+"/accept", nil)
		}()
	}
	wg.Wait()
	ok := 0
	for _, c := range codes {
		switch c {
		case 200:
			ok++
		case 404:
		default:
			t.Fatalf("unexpected status %d in %v", c, codes)
		}
	}
	if ok != 1 {
		t.Fatalf("%d accepts succeeded: %v", ok, codes)
	}
	if m := must[[]domain.OrgPerson](owner.client, 200, "GET", fmt.Sprintf("/api/orgs/%d/members", team.ID), nil); len(m) != 2 {
		t.Fatalf("members %+v", m)
	}
}

func TestWorkspaceHasThePlanOfItsOwner(t *testing.T) {
	e := newB2CEnv(t)
	admin := e.admin()
	owner, bob := e.person("Ayu"), e.person("Bob")
	team := newTeam(owner.client, "Tim")
	token, _ := invite(owner.client, team.ID, bob.email, "member")
	join(bob, token)

	// Free owner: the team is limited to 3 wallets, whoever creates them.
	ot, bt := owner.in(team.ID), bob.in(team.ID)
	for i := range 3 {
		ot.wallet(fmt.Sprintf("Kas %d", i), 0)
	}
	bt.expect(403, "POST", "/api/wallets", walletBody("Keempat"))
	if m := membershipOf(bt); m.Plan != domain.PlanFree || m.IsOwner || m.OrgID != team.ID || m.Usage[service.LimitWallets] != 3 {
		t.Fatalf("member's view of a free team %+v", m)
	}

	// Premium for the owner lifts the limit for everyone in the team, but not
	// for Bob's own personal workspace.
	admin.expect(201, "POST", fmt.Sprintf("/api/admin/users/%d/premium", owner.id), M{"note": "tim"})
	bt.expect(201, "POST", "/api/wallets", walletBody("Keempat"))
	if m := membershipOf(bt); m.Plan != domain.PlanPremium || m.IsOwner {
		t.Fatalf("member's view of a premium team %+v", m)
	}
	if m := membershipOf(bob.client); m.Plan != domain.PlanFree || !m.IsOwner {
		t.Fatalf("bob's personal workspace %+v", m)
	}
	if m := membershipOf(ot); !m.IsOwner || m.Plan != domain.PlanPremium {
		t.Fatalf("owner's view %+v", m)
	}
}

func TestTeamNotificationsNameTheWorkspaceAndReachEveryMember(t *testing.T) {
	e := newEnv(t)
	owner, bob := e.person("Ayu"), e.person("Bob")
	team := newTeam(owner.client, "Kos Mawar")
	token, _ := invite(owner.client, team.ID, bob.email, "member")
	join(bob, token)

	ot := owner.in(team.ID)
	w := ot.wallet("Kas", 60_000)
	cat := ot.category(model.TxExpense, "Makan")
	ot.expect(201, "POST", "/api/transactions", txBody(w.ID, cat, "expense", 30_000, "2026-01-15")) // 30.000 left: low
	if _, err := e.app.Notifications.Sweep(t.Context()); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]person{"owner": owner, "member": bob} {
		inbox := inboxOf(p.client)
		found := false
		for _, n := range inbox.Items {
			if n.Kind == service.NotifyLowBalance && strings.HasPrefix(n.Title, "[Kos Mawar] ") {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s did not get the team's low-balance notice: %+v", name, inbox.Items)
		}
	}
	// Sweeping again never repeats it.
	before := len(inboxOf(bob.client).Items)
	if _, err := e.app.Notifications.Sweep(t.Context()); err != nil {
		t.Fatal(err)
	}
	if after := len(inboxOf(bob.client).Items); after != before {
		t.Fatalf("second sweep added %d notices", after-before)
	}
}

func TestDeletingAWorkspaceDeletesItsData(t *testing.T) {
	e := newEnv(t)
	a := e.person("Ayu")
	team := newTeam(a.client, "Sementara")
	at := a.in(team.ID)
	at.wallet("Kas", 5000)
	a.wallet("Aman", 1000)
	a.expect(204, "DELETE", fmt.Sprintf("/api/orgs/%d", team.ID), nil)
	at.expect(403, "GET", "/api/wallets", nil)
	if w := must[[]model.Wallet](a.client, 200, "GET", "/api/wallets", nil); len(w) != 1 || w[0].Name != "Aman" {
		t.Fatalf("personal data after deleting the team: %v", w)
	}
}

func TestFrozenAccountsCannotChangeWorkspaces(t *testing.T) {
	e := newEnv(t)
	a := e.person("Ayu")
	team := newTeam(a.client, "Tim")
	e.admin().expect(200, "POST", fmt.Sprintf("/api/admin/users/%d/status", a.id), M{"status": "frozen", "reason": "Cek"})
	a.expect(200, "GET", "/api/orgs", nil)
	a.expect(403, "POST", "/api/orgs", M{"name": "Baru"})
	a.expect(403, "PUT", fmt.Sprintf("/api/orgs/%d", team.ID), M{"name": "Ganti"})
	a.in(team.ID).expect(403, "POST", "/api/wallets", walletBody("Kas"))
	a.in(team.ID).expect(200, "GET", "/api/wallets", nil)
}

// doHeader sends a request with a raw X-Org-Id value.
func (c *client) doHeader(method, path, org string) (int, []byte) {
	c.e.t.Helper()
	req, err := http.NewRequest(method, c.e.srv.URL+path, nil)
	if err != nil {
		c.e.t.Fatal(err)
	}
	req.Header.Set("X-Org-Id", org)
	resp, err := c.hc.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}
