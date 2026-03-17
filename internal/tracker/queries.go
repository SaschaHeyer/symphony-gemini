package tracker

// candidateIssuesQuery fetches issues in active states for a project.
// Uses slugId filter as required by SPEC Section 11.2.
const candidateIssuesQuery = `
query($projectSlug: String!, $states: [String!]!, $after: String) {
  issues(
    filter: {
      project: { slugId: { eq: $projectSlug } }
      state: { name: { in: $states } }
    }
    first: 50
    after: $after
    orderBy: createdAt
  ) {
    pageInfo {
      hasNextPage
      endCursor
    }
    nodes {
      id
      identifier
      title
      description
      priority
      state { name }
      branchName
      url
      labels { nodes { name } }
      relations {
        nodes {
          type
          relatedIssue {
            id
            identifier
            state { name }
          }
        }
      }
      createdAt
      updatedAt
    }
  }
}
`

// issueStatesByIDsQuery fetches current states for specific issue IDs.
// Uses [ID!] variable type as specified in SPEC Section 11.2.
const issueStatesByIDsQuery = `
query($ids: [ID!]!) {
  issues(filter: { id: { in: $ids } }) {
    nodes {
      id
      identifier
      state { name }
    }
  }
}
`

// issuesByStatesQuery fetches issues in specific states for a project.
// Used for startup terminal cleanup.
const issuesByStatesQuery = `
query($projectSlug: String!, $states: [String!]!, $after: String) {
  issues(
    filter: {
      project: { slugId: { eq: $projectSlug } }
      state: { name: { in: $states } }
    }
    first: 50
    after: $after
  ) {
    pageInfo {
      hasNextPage
      endCursor
    }
    nodes {
      id
      identifier
      title
      state { name }
    }
  }
}
`
