// Webpack config of the throw-away spike pages in this directory. Output goes to ./dist, which is
// not embedded into the otns binary (script/pack-web only packs templates/ and static/).
// Build from web/site with: npx webpack --config spike/webpack.config.js
const path = require('path');
const TerserPlugin = require('terser-webpack-plugin');

module.exports = {
    mode: 'production',
    context: __dirname,

    entry: {
        'pixi-three': './pixi-three.js',
    },

    output: {
        path: path.resolve(__dirname, 'dist'),
        filename: '[name].js',
        asyncChunks: false,
    },

    optimization: {
        minimizer: [new TerserPlugin({extractComments: false})],
    },

    performance: {
        hints: false,
    },

    resolve: {
        modules: [path.resolve(__dirname, '..', 'node_modules')],
        extensions: ['.mjs', '.js', '.json'],
    },

    module: {
        rules: [
            {
                test: /\.mjs$/,
                include: /node_modules/,
                type: 'javascript/auto'
            }
        ]
    }
};
