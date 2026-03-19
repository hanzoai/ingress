import { useContext, useMemo } from 'react'
import { Helmet } from 'react-helmet-async'

import { VersionContext } from 'contexts/version'

const PageTitle = ({ title }: { title?: string }) => {
  const { dashboardName } = useContext(VersionContext)

  const pageTitle = useMemo(
    () => `${title ? `${title} - ` : ''}Hanzo Ingress${dashboardName ? ` [${dashboardName}]` : ''}`,
    [dashboardName, title],
  )

  return (
    <Helmet>
      <title>{pageTitle}</title>
    </Helmet>
  )
}

export default PageTitle
