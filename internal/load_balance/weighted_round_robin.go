package load_balance

// 平滑加权轮询算法

type Server struct {
	Name          string
	Weight        int
	CurrentWeight int
}

type WeightedRoundRobin struct {
	servers     []Server
	totalWeight int
}

func (wrr *WeightedRoundRobin) AddServer(name string, weight int) {
	wrr.servers = append(wrr.servers, Server{
		Name:          name,
		Weight:        weight,
		CurrentWeight: 0,
	})
	wrr.totalWeight += weight
}

func (wrr *WeightedRoundRobin) GetServer() *Server {
	var res *Server
	for _, s := range wrr.servers {
		s.CurrentWeight += s.Weight
		if res == nil || res.CurrentWeight < s.CurrentWeight {
			res = &s
		}
	}
	res.CurrentWeight -= wrr.totalWeight
	return res
}
