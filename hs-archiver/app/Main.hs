module Main where

import Network.Wai.Handler.Warp (run)
import Smap.Api (smapApp)
import Smap.Db (initPool)
import System.Environment (getEnv)
import Control.Exception (catch, SomeException)
import System.IO (hSetBuffering, stdout, BufferMode(LineBuffering))

main :: IO ()
main = do
    hSetBuffering stdout LineBuffering
    -- Get connection string from environment or use default
    connStr <- getEnv "DB_CONN" `catch` \(_ :: SomeException) -> 
        return "host=localhost port=5432 user=archiver dbname=archiver password=archiver"
    
    putStrLn $ "Starting sMAP Archiver (Haskell) with connection: " ++ connStr
    
    pool <- initPool connStr
    
    putStrLn "Listening on port 8079..."
    run 8079 (smapApp pool)
