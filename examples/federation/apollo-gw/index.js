const { ApolloServer } = require('apollo-server');
const { ApolloGateway } = require('@apollo/gateway');

const gateway = new ApolloGateway({
    serviceList: [
        { name: 'accounts', url: 'http://localhost:4001/query' },
        { name: 'products', url: 'http://localhost:4002/query' },
        { name: 'reviews', url: 'http://localhost:4003/query' },
    ]
});

const server = new ApolloServer({
    gateway,
    // plugins: [
    //     {
    //         requestDidStart: ( requestContext ) => {
    //             if ( requestContext.request.http?.headers.has( 'x-apollo-tracing' ) ) {
    //                 return;
    //             }
    //             const query = requestContext.request.query?.replace( /\s+/g, ' ' ).trim();
    //             const variables = JSON.stringify( requestContext.request.variables );
    //             console.log( new Date().toISOString(), `- [Request Started] { query: ${ query }, variables: ${ variables }, operationName: ${ requestContext.request.operationName } }` );
    //         },
    //     },
    // ]
});

server.listen({
    host: '127.0.0.1',
    port: 4005
}).then(({ url }) => {
    console.log(`🚀 Gateway ready at ${url}`);
}).catch(err => {console.error(err)});
