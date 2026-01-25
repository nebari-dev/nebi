@README.md

Take a look at https://github.com/nebari-dev/nebi/issues/50 which was created based on the design docs referenced in there (present in the repo).  There are phase-0 through phase-6 branches after implementing each phase.  Take a look at those design docs in the docs/design folder locally. 

I've since tried out phase 6 branch and I found a lot of additional things we need to address in @./docs/edge-cases.txt.  We are going to address them in the current phase-7 branch.

I want you to split up the issues in edge-cases and start a bunch of agents to look into each of these using the /agents subcommand.  They shouldn't start work yet, but they should consider what I've discussed and consider possible solutions.  Assuming you use the correct slash agent subcommand, I'll be able to go and see their plans in those individual chats. Eventually I'll prompt them to write their findings in a local markdown document in the ./docs/issues folder.

They don't need to worry about any kind of backwards compatibility because there are no users yet.  Do you need any questions or clarifications before beginning?
