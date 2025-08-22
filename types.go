package twingate

type RemoteNetwork struct {
	ID   string `graphql:"id"`
	Name string `graphql:"name"`
}

type ResourceAddress struct {
	Value string `json:"value"`
}

type Resource struct {
	ID      string `graphql:"id"`
	Name    string `graphql:"name"`
	Address struct {
		Value string `graphql:"value"`
	} `graphql:"address"`
	Alias         *string `graphql:"alias"`
	RemoteNetwork struct {
		ID string `graphql:"id"`
	} `graphql:"remoteNetwork"`
}

type ResourceCreateInput struct {
	Name            string `json:"name"`
	Address         string `json:"address"`
	RemoteNetworkID string `json:"remoteNetworkId"`
	Alias           string `json:"alias,omitempty"`
}

type ResourceUpdateInput struct {
	ID      string  `json:"id"`
	Name    *string `json:"name,omitempty"`
	Address *string `json:"address,omitempty"`
	Alias   *string `json:"alias,omitempty"`
}

type RemoteNetworkCreateInput struct {
	Name string `json:"name"`
}

type RemoteNetworksQuery struct {
	RemoteNetworks struct {
		PageInfo struct {
			HasNextPage     bool    `json:"hasNextPage"`
			HasPreviousPage bool    `json:"hasPreviousPage"`
			StartCursor     *string `json:"startCursor"`
			EndCursor       *string `json:"endCursor"`
		} `json:"pageInfo"`
		Edges []struct {
			Cursor string        `json:"cursor"`
			Node   RemoteNetwork `json:"node"`
		} `json:"edges"`
		TotalCount int `json:"totalCount"`
	} `graphql:"remoteNetworks(first: $first)"`
}

type ResourcesQuery struct {
	Resources struct {
		PageInfo struct {
			HasNextPage     bool    `json:"hasNextPage"`
			HasPreviousPage bool    `json:"hasPreviousPage"`
			StartCursor     *string `json:"startCursor"`
			EndCursor       *string `json:"endCursor"`
		} `json:"pageInfo"`
		Edges []struct {
			Cursor string   `json:"cursor"`
			Node   Resource `json:"node"`
		} `json:"edges"`
		TotalCount int `json:"totalCount"`
	} `graphql:"resources(first: $first)"`
}

type ResourceCreateMutation struct {
	ResourceCreate struct {
		OK     bool      `graphql:"ok"`
		Error  *string   `graphql:"error"`
		Entity *Resource `graphql:"entity"`
	} `graphql:"resourceCreate(address: $address, name: $name, remoteNetworkId: $remoteNetworkId)"`
}

type ResourceUpdateMutation struct {
	ResourceUpdate struct {
		OK     bool      `graphql:"ok"`
		Error  *string   `graphql:"error"`
		Entity *Resource `graphql:"entity"`
	} `graphql:"resourceUpdate(id: $id, name: $name, address: $address, alias: $alias)"`
}

type RemoteNetworkCreateMutation struct {
	RemoteNetworkCreate struct {
		OK     bool           `graphql:"ok"`
		Error  *string        `graphql:"error"`
		Entity *RemoteNetwork `graphql:"entity"`
	} `graphql:"remoteNetworkCreate(name: $name)"`
}

type ResourceMapping struct {
	Name    string
	Alias   *string
	Address string
}
