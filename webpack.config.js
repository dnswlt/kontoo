const MiniCssExtractPlugin = require('mini-css-extract-plugin');
const path = require('path');
const mode = process.env.NODE_ENV || 'development';

module.exports = {
    mode: mode,
    entry: './resources/main.js',
    output: {
        path: path.resolve(__dirname, 'dist'), // Output directory for the bundled JavaScript
        filename: 'bundle.js'
    },
    module: {
        rules: [
            {
                test: /\.css$/,
                use: [
                    mode === 'development' ? 'style-loader' : MiniCssExtractPlugin.loader,
                    'css-loader',
                ],
            }
            // Optional: Use a loader like `eslint-loader` for linting
            // {
            //     enforce: 'pre',
            //     test: /\.js$/,
            //     exclude: /node_modules/,
            //     use: ['eslint-loader']
            // }
        ]
    },
    plugins: [
        // Add MiniCssExtractPlugin only in production mode
        ...(mode === 'production' ? [new MiniCssExtractPlugin({
            filename: '[name].css',
            chunkFilename: '[id].css',
        })] : []),
    ],
};