// Makes a machine while running: one job may run git, one may run deploy.
const text = `
flow: made
stages:
  status: { job: { script: ./run.js, input: { what: git status }, allow: [exec:git] } }
  deploy: { job: { script: ./run.js, input: { what: deploy }, allow: [exec:deploy] } }
  told: { job: { script: ./run.js, input: { what: "\${stages.deploy.error.error}" } } }
output: { status: "\${stages.status.output}", deploy: "\${stages.told.output}" }
`
return machines.define(text, { files: { "./run.js": "return 'ran ' + input.what" } })
