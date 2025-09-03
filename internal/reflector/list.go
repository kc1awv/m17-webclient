package reflector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/kc1awv/m17-webclient/internal/logger"
)

type ReflectorInfo struct {
	Designator string `json:"designator"`
	Name       string `json:"name"`
	Address    string `json:"address"`
	Slug       string `json:"slug"`
	Legacy     bool   `json:"legacy"`
}

type hostfile struct {
	Reflectors []hostfileReflector `json:"reflectors"`
}

type hostfileReflector struct {
	Designator     string `json:"designator"`
	Name           string `json:"name"`
	IPv4           string `json:"ipv4"`
	IPv6           string `json:"ipv6"`
	Domain         string `json:"domain"`
	Modules        string `json:"modules"`
	SpecialModules string `json:"special_modules"`
	Port           int    `json:"port"`
	Source         string `json:"source"`
	URL            string `json:"url"`
	Version        string `json:"version"`
	Legacy         bool   `json:"legacy"`
}

type cachedModules struct {
	Modules []string
}

type ListStore struct {
	reflectorList []ReflectorInfo
	designatorMap map[string]string
	mu            sync.RWMutex

	moduleCache map[string]cachedModules
	moduleMu    sync.RWMutex

	hostFilePath    string
	hostFileModTime time.Time
}

func NewListStore() *ListStore {
	return &ListStore{
		moduleCache: make(map[string]cachedModules),
	}
}

func (ls *ListStore) Init() {
	ls.hostFilePath = os.Getenv("M17_HOSTFILE")
	if ls.hostFilePath == "" {
		log.Warn("M17_HOSTFILE not set; reflector list will be empty")
	}
}

func loadHostFile(ctx context.Context, path string, modTime time.Time) (*hostfile, time.Time, error) {
	if err := ctx.Err(); err != nil {
		return nil, time.Time{}, err
	}

	stat, err := os.Stat(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	if !stat.ModTime().After(modTime) {
		return nil, stat.ModTime(), nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer f.Close()

	var hf hostfile
	if err := json.NewDecoder(f).Decode(&hf); err != nil {
		return nil, time.Time{}, err
	}
	return &hf, stat.ModTime(), nil
}

func (ls *ListStore) FetchReflectors(ctx context.Context) {
	if ls.hostFilePath == "" || ctx.Err() != nil {
		return
	}

	hf, modTime, err := loadHostFile(ctx, ls.hostFilePath, ls.hostFileModTime)
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			log.Error("Error loading host file", "err", err, "path", ls.hostFilePath)
		}
		return
	}
	if hf == nil {
		return
	}

	var list []ReflectorInfo
	newModuleCache := make(map[string]cachedModules)
	newDesignatorMap := make(map[string]string)

	for _, r := range hf.Reflectors {
		host := r.IPv4
		if host == "" {
			host = r.Domain
		}
		if host == "" && r.IPv6 != "" {
			host = fmt.Sprintf("[%s]", r.IPv6)
		}
		if host == "" {
			continue
		}
		addr := fmt.Sprintf("%s:%d", host, r.Port)
		slug := strings.ToLower(r.Designator)

		list = append(list, ReflectorInfo{
			Designator: r.Designator,
			Name:       r.Name,
			Address:    addr,
			Slug:       slug,
			Legacy:     r.Legacy,
		})

		newDesignatorMap[addr] = r.Designator

		if r.Modules != "" {
			mods := []string{}
			for _, m := range r.Modules {
				if m >= 'A' && m <= 'Z' {
					mods = append(mods, string(m))
				}
			}
			sort.Strings(mods)
			if len(mods) > 0 {
				newModuleCache[slug] = cachedModules{Modules: mods}
			}
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].Designator < list[j].Designator
	})

	ls.mu.Lock()
	ls.reflectorList = list
	ls.designatorMap = newDesignatorMap
	ls.mu.Unlock()

	ls.moduleMu.Lock()
	ls.moduleCache = newModuleCache
	ls.moduleMu.Unlock()

	ls.hostFileModTime = modTime
	log.Info("Updated reflector list", "count", len(list))
}

func (ls *ListStore) FetchModules(slug string) []string {
	ls.moduleMu.RLock()
	cached, ok := ls.moduleCache[slug]
	ls.moduleMu.RUnlock()
	if ok {
		return append([]string(nil), cached.Modules...)
	}
	return []string{}
}

func (ls *ListStore) GetReflectors() []ReflectorInfo {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return append([]ReflectorInfo(nil), ls.reflectorList...)
}

func (ls *ListStore) LookupDesignator(addr string) string {
	ls.mu.RLock()
	d := ls.designatorMap[addr]
	ls.mu.RUnlock()
	return d
}

func (ls *ListStore) StartReflectorUpdater(ctx context.Context) {
	ls.FetchReflectors(ctx)
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ls.FetchReflectors(ctx)
			}
		}
	}()
}
