package plan

import (
	"github.com/helmwave/helmwave/pkg/helper"
	"github.com/helmwave/helmwave/pkg/release"
	"github.com/helmwave/helmwave/pkg/repo"
	log "github.com/sirupsen/logrus"
	"os"
	"strings"
)

type buildOptions int

const (
	UniqNames buildOptions = iota
	Release
	Values
	Repositories
	Manifests
)

// Build Принимает на вход ямл и опции по работе с ним.
func (p *Plan) Build(yml string, tags []string, matchAll bool) error {
	// Create Body
	body, err := NewBody(yml)
	if err != nil {
		return err
	}
	p.body = body

	// Build Releases
	p.body.Releases = buildReleases(tags, p.body.Releases, matchAll)

	// Build Values
	err = buildValues(p.body.Releases, p.dir)
	if err != nil {
		return err
	}

	// Build Repositories
	p.body.Repositories = buildRepo(p.body.Releases, p.body.Repositories)

	return nil
}

func buildReleases(tags []string, releases []*release.Config, matchAll bool) (plan []*release.Config) {
	if len(tags) == 0 {
		return releases
	}

	releasesMap := make(map[string]*release.Config)

	for _, r := range releases {
		releasesMap[r.UniqName()] = r
	}

	for _, r := range releases {
		if checkTagInclusion(tags, r.Tags, matchAll) {
			plan = addToPlan(plan, r, releasesMap)
		}
	}

	return plan
}

func addToPlan(plan []*release.Config, release *release.Config, releases map[string]*release.Config) []*release.Config {
	if release.In(plan) {
		return plan
	}

	r := append(plan, release)

	for _, depName := range release.DependsOn {
		if dep, ok := releases[depName]; ok {
			r = addToPlan(r, dep, releases)
		} else {
			log.Warnf("cannot find dependency %s in available releases, skipping it", depName)
		}
	}

	return r
}

// checkTagInclusion checks where any of release tags are included in target tags.
func checkTagInclusion(targetTags, releaseTags []string, matchAll bool) bool {
	for _, t := range targetTags {
		contains := helper.Contains(t, releaseTags)
		if matchAll && !contains {
			return false
		}
		if !matchAll && contains {
			return true
		}
	}

	return matchAll
}

func buildValues(releases []*release.Config, dir string) error {
	for i, rel := range releases {
		err := rel.RenderValues(dir)
		if err != nil {
			return err
		}

		releases[i].Values = rel.Values
		log.WithFields(log.Fields{
			"release":   rel.Name,
			"namespace": rel.Options.Namespace,
			"values":    releases[i].Values,
		}).Debug("🐞 Render Values")
	}

	return nil
}

func buildRepo(releases []*release.Config, repositories []*repo.Config) (plan []*repo.Config) {
	all := getRepositories(releases)

	for _, a := range all {
		found := false
		for _, b := range repositories {
			if a == b.Name {
				found = true
				if !b.InByName(plan) {
					plan = append(plan, b)
					log.Infof("🗄 %q has been added to the plan", a)
				}
			}
		}

		if !found {
			log.Errorf("🗄 %q not found ", a)
		}
	}

	return plan
}

// getRepositories for releases
func getRepositories(releases []*release.Config) (repos []string) {
	for _, rel := range releases {
		rep := strings.Split(rel.Chart, "/")[0]
		deps, _ := rel.ReposDeps()

		all := deps
		if repoIsLocal(rep) {
			log.Infof("🗄 %q is local repo", rep)
		} else {
			all = append(all, rep)
		}

		for _, r := range all {
			if !helper.Contains(r, repos) {
				repos = append(repos, r)
			}
		}
	}

	return repos
}

// repoIsLocal
func repoIsLocal(repo string) bool {
	if repo == "" {
		return true
	}

	stat, err := os.Stat(repo)
	if (err == nil || !os.IsNotExist(err)) && stat.IsDir() {
		return true
	}

	return false
}
