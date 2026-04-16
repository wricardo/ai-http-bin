package sdk

import "context"

// Token mirrors the GraphQL Token type.
type Token struct {
	ID                 string  `json:"id"`
	AgentID            *string `json:"agentId,omitempty"`
	URL                string  `json:"url"`
	IP                 string  `json:"ip"`
	UserAgent          string  `json:"userAgent"`
	CreatedAt          string  `json:"createdAt"`
	ExpiresAt          string  `json:"expiresAt"`
	RequestCount       int     `json:"requestCount"`
	DefaultStatus      int     `json:"defaultStatus"`
	DefaultContent     string  `json:"defaultContent"`
	DefaultContentType string  `json:"defaultContentType"`
	Timeout            int     `json:"timeout"`
	Cors               bool    `json:"cors"`
	Script             string  `json:"script"`
}

// Request mirrors the GraphQL Request type.
type Request struct {
	ID        string `json:"id"`
	TokenID   string `json:"tokenId"`
	Method    string `json:"method"`
	URL       string `json:"url"`
	Hostname  string `json:"hostname"`
	Path      string `json:"path"`
	Headers   string `json:"headers"`
	Query     string `json:"query"`
	Body      string `json:"body"`
	FormData  string `json:"formData"`
	IP        string `json:"ip"`
	UserAgent string `json:"userAgent"`
	CreatedAt string `json:"createdAt"`
}

type RequestPage struct {
	Data        []Request `json:"data"`
	Total       int       `json:"total"`
	PerPage     int       `json:"perPage"`
	CurrentPage int       `json:"currentPage"`
	IsLastPage  bool      `json:"isLastPage"`
	From        int       `json:"from"`
	To          int       `json:"to"`
}

type GlobalVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type CreateTokenInput struct {
	DefaultStatus      *int
	DefaultContent     *string
	DefaultContentType *string
	Timeout            *int
	Cors               *bool
	Script             *string
}

type UpdateTokenInput struct {
	DefaultStatus      *int
	DefaultContent     *string
	DefaultContentType *string
	Timeout            *int
	Cors               *bool
}

type RequestsOptions struct {
	Page    *int
	PerPage *int
	Sorting *string // oldest|newest
}

func (c *Client) CreateToken(ctx context.Context, in CreateTokenInput) (*Token, error) {
	const query = `mutation($defaultStatus:Int,$defaultContent:String,$defaultContentType:String,$timeout:Int,$cors:Boolean,$script:String){createToken(defaultStatus:$defaultStatus,defaultContent:$defaultContent,defaultContentType:$defaultContentType,timeout:$timeout,cors:$cors,script:$script){id agentId url ip userAgent createdAt requestCount defaultStatus defaultContent defaultContentType timeout cors script}}`
	vars := map[string]any{
		"defaultStatus":      in.DefaultStatus,
		"defaultContent":     in.DefaultContent,
		"defaultContentType": in.DefaultContentType,
		"timeout":            in.Timeout,
		"cors":               in.Cors,
		"script":             in.Script,
	}
	var out struct {
		CreateToken Token `json:"createToken"`
	}
	if err := c.do(ctx, query, vars, &out); err != nil {
		return nil, err
	}
	return &out.CreateToken, nil
}

func (c *Client) UpdateToken(ctx context.Context, id string, in UpdateTokenInput) (*Token, error) {
	const query = `mutation($id:ID!,$defaultStatus:Int,$defaultContent:String,$defaultContentType:String,$timeout:Int,$cors:Boolean){updateToken(id:$id,defaultStatus:$defaultStatus,defaultContent:$defaultContent,defaultContentType:$defaultContentType,timeout:$timeout,cors:$cors){id agentId url ip userAgent createdAt requestCount defaultStatus defaultContent defaultContentType timeout cors script}}`
	vars := map[string]any{
		"id":                 id,
		"defaultStatus":      in.DefaultStatus,
		"defaultContent":     in.DefaultContent,
		"defaultContentType": in.DefaultContentType,
		"timeout":            in.Timeout,
		"cors":               in.Cors,
	}
	var out struct {
		UpdateToken *Token `json:"updateToken"`
	}
	if err := c.do(ctx, query, vars, &out); err != nil {
		return nil, err
	}
	return out.UpdateToken, nil
}

func (c *Client) SetScript(ctx context.Context, id, script string) (*Token, error) {
	const query = `mutation($id:ID!,$script:String!){setScript(id:$id,script:$script){id agentId url ip userAgent createdAt requestCount defaultStatus defaultContent defaultContentType timeout cors script}}`
	var out struct {
		SetScript Token `json:"setScript"`
	}
	if err := c.do(ctx, query, map[string]any{"id": id, "script": script}, &out); err != nil {
		return nil, err
	}
	return &out.SetScript, nil
}

func (c *Client) ToggleCors(ctx context.Context, id string) (bool, error) {
	const query = `mutation($id:ID!){toggleCors(id:$id)}`
	var out struct {
		ToggleCors bool `json:"toggleCors"`
	}
	if err := c.do(ctx, query, map[string]any{"id": id}, &out); err != nil {
		return false, err
	}
	return out.ToggleCors, nil
}

func (c *Client) DeleteToken(ctx context.Context, id string) (bool, error) {
	const query = `mutation($id:ID!){deleteToken(id:$id)}`
	var out struct {
		DeleteToken bool `json:"deleteToken"`
	}
	if err := c.do(ctx, query, map[string]any{"id": id}, &out); err != nil {
		return false, err
	}
	return out.DeleteToken, nil
}

func (c *Client) ClaimToken(ctx context.Context, id string) (*Token, error) {
	const query = `mutation($id:ID!){claimToken(id:$id){id agentId url ip userAgent createdAt requestCount defaultStatus defaultContent defaultContentType timeout cors script}}`
	var out struct {
		ClaimToken *Token `json:"claimToken"`
	}
	if err := c.do(ctx, query, map[string]any{"id": id}, &out); err != nil {
		return nil, err
	}
	return out.ClaimToken, nil
}

func (c *Client) Token(ctx context.Context, id string) (*Token, error) {
	const query = `query($id:ID!){token(id:$id){id agentId url ip userAgent createdAt requestCount defaultStatus defaultContent defaultContentType timeout cors script}}`
	var out struct {
		Token *Token `json:"token"`
	}
	if err := c.do(ctx, query, map[string]any{"id": id}, &out); err != nil {
		return nil, err
	}
	return out.Token, nil
}

func (c *Client) Tokens(ctx context.Context) ([]Token, error) {
	const query = `query{tokens{id agentId url ip userAgent createdAt requestCount defaultStatus defaultContent defaultContentType timeout cors script}}`
	var out struct {
		Tokens []Token `json:"tokens"`
	}
	if err := c.do(ctx, query, nil, &out); err != nil {
		return nil, err
	}
	return out.Tokens, nil
}

func (c *Client) Request(ctx context.Context, id string) (*Request, error) {
	const query = `query($id:ID!){request(id:$id){id tokenId method url hostname path headers query body formData ip userAgent createdAt}}`
	var out struct {
		Request *Request `json:"request"`
	}
	if err := c.do(ctx, query, map[string]any{"id": id}, &out); err != nil {
		return nil, err
	}
	return out.Request, nil
}

func (c *Client) Requests(ctx context.Context, tokenID string, opts RequestsOptions) (*RequestPage, error) {
	const query = `query($tokenId:ID!,$page:Int,$perPage:Int,$sorting:String){requests(tokenId:$tokenId,page:$page,perPage:$perPage,sorting:$sorting){data{id tokenId method url hostname path headers query body formData ip userAgent createdAt} total perPage currentPage isLastPage from to}}`
	vars := map[string]any{
		"tokenId": tokenID,
		"page":    opts.Page,
		"perPage": opts.PerPage,
		"sorting": opts.Sorting,
	}
	if opts.Page == nil {
		vars["page"] = defaultPage
	}
	if opts.PerPage == nil {
		vars["perPage"] = defaultPerPage
	}
	if opts.Sorting == nil {
		vars["sorting"] = defaultSortMode
	}
	var out struct {
		Requests RequestPage `json:"requests"`
	}
	if err := c.do(ctx, query, vars, &out); err != nil {
		return nil, err
	}
	return &out.Requests, nil
}

func (c *Client) DeleteRequest(ctx context.Context, id string) (bool, error) {
	const query = `mutation($id:ID!){deleteRequest(id:$id)}`
	var out struct {
		DeleteRequest bool `json:"deleteRequest"`
	}
	if err := c.do(ctx, query, map[string]any{"id": id}, &out); err != nil {
		return false, err
	}
	return out.DeleteRequest, nil
}

func (c *Client) ClearRequests(ctx context.Context, tokenID string) (bool, error) {
	const query = `mutation($tokenId:ID!){clearRequests(tokenId:$tokenId)}`
	var out struct {
		ClearRequests bool `json:"clearRequests"`
	}
	if err := c.do(ctx, query, map[string]any{"tokenId": tokenID}, &out); err != nil {
		return false, err
	}
	return out.ClearRequests, nil
}

func (c *Client) GlobalVars(ctx context.Context) ([]GlobalVar, error) {
	const query = `query{globalVars{key value}}`
	var out struct {
		GlobalVars []GlobalVar `json:"globalVars"`
	}
	if err := c.do(ctx, query, nil, &out); err != nil {
		return nil, err
	}
	return out.GlobalVars, nil
}

func (c *Client) SetGlobalVar(ctx context.Context, key, value string) (*GlobalVar, error) {
	const query = `mutation($key:String!,$value:String!){setGlobalVar(key:$key,value:$value){key value}}`
	var out struct {
		SetGlobalVar GlobalVar `json:"setGlobalVar"`
	}
	if err := c.do(ctx, query, map[string]any{"key": key, "value": value}, &out); err != nil {
		return nil, err
	}
	return &out.SetGlobalVar, nil
}

func (c *Client) DeleteGlobalVar(ctx context.Context, key string) (bool, error) {
	const query = `mutation($key:String!){deleteGlobalVar(key:$key)}`
	var out struct {
		DeleteGlobalVar bool `json:"deleteGlobalVar"`
	}
	if err := c.do(ctx, query, map[string]any{"key": key}, &out); err != nil {
		return false, err
	}
	return out.DeleteGlobalVar, nil
}
