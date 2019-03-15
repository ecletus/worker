package worker

import (
	"github.com/moisespsena/go-i18n-modular/i18nmod"
	"github.com/moisespsena/go-path-helpers"
)

var (
	PREFIX    = path_helpers.GetCalledDir()
	I18NGROUP = i18nmod.PkgToGroup(PREFIX)
)
