package worker

import (
	"github.com/moisespsena/go-i18n-modular/i18nmod"
	"github.com/aghape/helpers"
)

var (
	PREFIX    = helpers.GetCalledDir()
	I18NGROUP = i18nmod.PkgToGroup(PREFIX)
)
